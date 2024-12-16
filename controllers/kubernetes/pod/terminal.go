package pod

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/hack"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/360yun/sockjs-go/sockjs"
	"github.com/gin-gonic/gin"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// 用于处理终端的输入输出和大小调整
type PtyHandler interface {
	io.Reader
	io.Writer
	remotecommand.TerminalSizeQueue
}

// 终端会话
type TerminalSession struct {
	id            string                          // 会话 ID
	sockJSSession sockjs.Session                  // WebSocket 会话
	sizeChan      chan remotecommand.TerminalSize // 用于处理终端尺寸变化的通道
}

// TerminalMessage 定义了客户端发送的终端消息结构
// 包括操作类型（Op）、数据（Data）、会话 ID（SessionID）和终端的行列数（Rows, Cols）
type TerminalMessage struct {
	Op, Data, SessionID string
	Rows, Cols          uint16
}

// TerminalResult 是终端会话的结果，包含会话、token、集群、命名空间、Pod、容器等信息
type TerminalResult struct {
	SessionId string `json:"sessionId,omitempty"`
	Token     string `json:"token,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Pod       string `json:"pod,omitempty"`
	Container string `json:"container,omitempty"`
	Cmd       string `json:"cmd,omitempty"`
}

// 获取终端的尺寸（行列数），从 sizeChan 通道获取
func (t TerminalSession) Next() *remotecommand.TerminalSize {
	select {
	case size := <-t.sizeChan:
		return &size
	}
}

// Read 从 WebSocket 会话接收数据，解析消息并处理终端输入（stdin）或终端尺寸调整（resize）
func (t TerminalSession) Read(p []byte) (int, error) {
	m, err := t.sockJSSession.Recv()
	if err != nil {
		return 0, err
	}

	var msg TerminalMessage
	if err := json.Unmarshal([]byte(m), &msg); err != nil {
		klog.Warningf("read msg (%s) from client error.%v", string(p), err)
		return 0, err
	}

	// 根据消息的操作类型进行处理
	switch msg.Op {
	case "stdin":
		// 将客户端输入数据复制到 p 缓冲区并返回
		return copy(p, msg.Data), nil
	case "resize":
		// 将终端尺寸变化发送到 sizeChan
		t.sizeChan <- remotecommand.TerminalSize{msg.Cols, msg.Rows}
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown message type '%s'", msg.Op)
	}
}

// 将数据（如容器输出）写入 WebSocket 会话，发送给客户端
func (t TerminalSession) Write(p []byte) (int, error) {
	msg, err := json.Marshal(TerminalMessage{
		Op:   "stdout",
		Data: string(p),
	})
	if err != nil {
		return 0, err
	}

	// 通过 WebSocket 会话发送数据
	if err = t.sockJSSession.Send(string(msg)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// 关闭 WebSocket 会话，并记录日志。
func (t TerminalSession) Close(status uint32, reason string) {
	t.sockJSSession.Close(status, reason)
	klog.Infof("close socket (%s). %d, %s", t.id, status, reason)
}

// 处理客户端发来的ws建立请求
func handleTerminalSession(session sockjs.Session) {
	var (
		buf string
		err error
		msg TerminalMessage
	)

	// 接收客户端发送的消息
	if buf, err = session.Recv(); err != nil {
		klog.Errorf("handleTerminalSession: can't Recv: %v", err)
		return
	}

	// 解析客户端发送的消息
	if err = json.Unmarshal([]byte(buf), &msg); err != nil {
		klog.Errorf("handleTerminalSession: can't UnMarshal (%v): %s", err, buf)
		return
	}

	// 检查消息的类型是否为 bind，如果不是，记录错误并返回
	if msg.Op != "bind" {
		klog.Errorf("handleTerminalSession: expected 'bind' message, got: %s", buf)
		return
	}

	var tr TerminalResult
	if err := json.Unmarshal(hack.Slice(msg.Data), &tr); err != nil {
		klog.Errorf("handleTerminalResult: can't UnMarshal (%v): %s", err, buf)
		return
	}

	// 验证客户端传来的 token 是否有效
	err = checkShellToken(tr.Token, tr.Namespace, tr.Pod)
	if err != nil {
		klog.Error(http.StatusBadRequest, fmt.Sprintf("token (%s) not valid %v.", tr.Token, err))
		return
	}

	manager, err := client.Manager(tr.Cluster)
	if err == nil {
		ts := TerminalSession{
			id:            tr.SessionId,
			sockJSSession: session,
			sizeChan:      make(chan remotecommand.TerminalSize),
		}
		go WaitForTerminal(manager.Client, manager.Config, ts, tr.Namespace, tr.Pod, tr.Container, "")
		return
	} else {
		klog.Error(http.StatusBadRequest, fmt.Sprintf("%s %v", tr.Cluster, err))
		return
	}
}

func CreateAttachHandler(path string) http.Handler {
	return sockjs.NewHandler(path, sockjs.DefaultOptions, handleTerminalSession)
}

// @Title Create terminal
// @Param	cmd		query 	string	true		"the cmd you want to exec."
// @Param	container		query 	string	true		"the container name."
// @Description create container terminal
// @router /:pod/terminal/namespaces/:namespace/clusters/:cluster [post]
func Terminal(c *gin.Context) {
	cluster := c.Param("cluster")
	pod := c.Param("pod")
	container := c.DefaultQuery("container", "")
	namespace := c.Param("namespace")
	cmd := c.DefaultQuery("cmd", "")

	if pod == "" || container == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pod and container are required!"})
		return
	}

	sessionId, err := genTerminalSessionId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	klog.V(2).Infof("sessionId: %s", sessionId)

	result := TerminalResult{
		SessionId: sessionId,
		Token:     generateToken(namespace, pod),
		Cluster:   cluster,
		Namespace: namespace,
		Pod:       pod,
		Container: container,
		Cmd:       cmd,
	}

	klog.V(2).Infof("result: %+v", result)

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// token生成规则
// 1. 拼接namespace、podName、unixtime(加600秒，十分钟期限)，平台appkey，并进行md5加密操作
// 2. 取生成的32位加密字符串第12-20位，于unixtime进行拼接生成token
func generateToken(namespace, pod string) string {
	appKey := config.Conf.App.AppKey
	endTime := time.Now().Unix() + 60*10
	rawTokenKey := namespace + pod + strconv.FormatInt(endTime, 10) + appKey
	md5Hash := md5.New()
	md5Hash.Write([]byte(rawTokenKey))
	cipher := md5Hash.Sum(nil)
	cipherStr := hex.EncodeToString(cipher)
	return cipherStr[12:20] + strconv.FormatInt(endTime, 10)
}

func checkShellToken(token string, namespace string, podName string) error {
	endTimeRaw := []rune(token)
	var endTime int64
	var endTimeStr string
	var err error

	if len(endTimeRaw) > 8 {
		endTimeStr = string(endTimeRaw[8:])
		endTime, err = strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			return err
		}
	}
	ntime := time.Now().Unix()

	if ntime > endTime {
		return errors.New("token time expired")
	}

	rawToken := namespace + podName + endTimeStr + config.Conf.App.AppKey

	md5Ctx := md5.New()
	md5Ctx.Write([]byte(rawToken))
	cipherToken := hex.EncodeToString(md5Ctx.Sum(nil))

	checkToken := string([]rune(cipherToken)[12:20]) + endTimeStr
	if checkToken != token {
		return errors.New("token not match")
	}
	return nil
}

// 开始建立ws连接
// Kubernetes 中启动进程并通过 ptyHandler 进行流交互
func startProcess(k8sClient *kubernetes.Clientset, cfg *rest.Config, cmd []string, ptyHandler PtyHandler, namespace, pod, container string) error {
	req := k8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             ptyHandler,
		Stdout:            ptyHandler,
		Stderr:            ptyHandler,
		TerminalSizeQueue: ptyHandler,
		Tty:               true,
	})
	if err != nil {
		return err
	}

	return nil
}

func genTerminalSessionId() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	id := make([]byte, hex.EncodedLen(len(bytes)))
	hex.Encode(id, bytes)
	return string(id), nil
}

func isValidShell(validShells []string, shell string) bool {
	for _, validShell := range validShells {
		if validShell == shell {
			return true
		}
	}
	return false
}

// WaitForTerminal 等待并启动一个终端会话，如果指定的 shell 有效，启动指定 shell 的进程。
// 如果指定的 shell 无效，则尝试使用有效的 shell（如 bash 或 sh）启动终端会话。
// 如果启动过程成功，关闭终端会话，否则返回错误信息。
func WaitForTerminal(k8sClient *kubernetes.Clientset, cfg *rest.Config, ts TerminalSession, namespace, pod, container, cmd string) {
	var err error
	// 定义 shell 类型
	validShells := []string{"bash", "sh"}

	if isValidShell(validShells, cmd) {
		cmds := []string{cmd}
		err = startProcess(k8sClient, cfg, cmds, ts, namespace, pod, container)
	} else {
		for _, testShell := range validShells {
			cmd := []string{testShell}
			if err = startProcess(k8sClient, cfg, cmd, ts, namespace, pod, container); err == nil {
				break
			}
		}
	}

	if err != nil {
		// 启动失败，关闭并返回错误
		ts.Close(2, err.Error())
		return
	}

	// 启动成功，关闭终端并返回 "Process exited"
	ts.Close(1, "Process exited")
}
