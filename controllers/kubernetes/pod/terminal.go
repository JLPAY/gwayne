package pod

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/360yun/sockjs-go/sockjs"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/hack"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
)

// 用于处理终端的输入输出和大小调整
type PtyHandler interface {
	io.Reader
	io.Writer
	remotecommand.TerminalSizeQueue
}

// 终端会话
type TerminalSession struct {
	id             string                          // 会话 ID
	sockJSSession  sockjs.Session                  // WebSocket 会话
	sizeChan       chan remotecommand.TerminalSize // 用于处理终端尺寸变化的通道
	user           *models.User                    // 用户信息
	commandBuffer  string                          // 命令缓冲区，用于累积完整命令
	lastCommand    string                          // 上一个命令，用于防止重复执行被拦截的命令
	commandBlocked bool                            // 标记上一个命令是否被拦截
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

// Shell检测缓存
type ShellCache struct {
	shell     string
	timestamp time.Time
}

var (
	shellCacheMap = make(map[string]*ShellCache)
	shellCacheMux sync.RWMutex
	shellCacheTTL = 5 * time.Minute // 5分钟缓存

	// sessionUserMap 存储 sessionId -> user 的映射
	sessionUserMap = make(map[string]*models.User)
	sessionUserMux sync.RWMutex
	sessionUserTTL = 10 * time.Minute // 10分钟过期
)

// 获取终端的尺寸（行列数），从 sizeChan 通道获取
func (t *TerminalSession) Next() *remotecommand.TerminalSize {
	size := <-t.sizeChan
	return &size
}

// Read 从 WebSocket 会话接收数据，解析消息并处理终端输入（stdin）或终端尺寸调整（resize）
func (t *TerminalSession) Read(p []byte) (int, error) {
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
		// 命令拦截检查
		if t.user != nil && !t.user.Admin {
			// 检查是否输入了换行符（命令结束）
			hasNewline := strings.Contains(msg.Data, "\n") || strings.Contains(msg.Data, "\r")

			if hasNewline {
				// 累积命令到缓冲区（包括换行符）
				t.commandBuffer += msg.Data

				// 提取完整命令（去除换行符和前后空格）
				command := strings.TrimSpace(strings.TrimRight(t.commandBuffer, "\n\r"))

				klog.Infof("Terminal command detected: '%s' (user: %s, admin: %v, sessionId: %s)",
					command, t.user.Name, t.user.Admin, t.id)

				// 如果命令为空，允许通过（用于清屏等操作）
				if command == "" {
					t.commandBuffer = ""
					t.commandBlocked = false
					t.lastCommand = ""
					// 只发送换行符，不重复发送命令
					return copy(p, msg.Data), nil
				}

				// 检查是否是重复执行被拦截的命令
				if t.commandBlocked && command == t.lastCommand {
					klog.Warningf("Command '%s' was already blocked, preventing re-execution", command)
					errorMsg, _ := json.Marshal(TerminalMessage{
						Op:   "stdout",
						Data: "\r\n\x1b[31m[命令被阻止]\x1b[0m Command Permission Denied: command was already blocked\r\n",
					})
					t.sockJSSession.Send(string(errorMsg))
					t.commandBuffer = ""
					// 阻止发送换行符，防止命令执行
					return 0, nil
				}

				// 检查命令是否被禁止
				if err := checkCommandPermission(t.user, command); err != nil {
					klog.Warningf("Command blocked: '%s' for user %s, reason: %v", command, t.user.Name, err)

					// 命令字符可能已经发送到容器，需要清除容器输入缓冲区
					// 发送 Ctrl+U (清除到行首) 来清除已输入的命令
					// Ctrl+U 的 ASCII 码是 0x15 (21)，在大多数 shell 中会清除当前行
					clearSequence := []byte{0x15} // Ctrl+U

					// 先发送清除序列到容器，清除输入缓冲区中的命令
					if len(p) >= len(clearSequence) {
						copy(p, clearSequence)

						// 记录被拦截的命令
						t.lastCommand = command
						t.commandBlocked = true
						t.commandBuffer = ""

						// 同步发送错误消息给前端，确保用户能看到提示
						// 使用 stdout 而不是 stderr，这样更明显
						errorMsg, _ := json.Marshal(TerminalMessage{
							Op:   "stdout",
							Data: "\r\n\x1b[31m[命令被阻止]\x1b[0m Command Permission Denied: " + err.Error() + "\r\n",
						})
						if sendErr := t.sockJSSession.Send(string(errorMsg)); sendErr != nil {
							klog.Errorf("Failed to send error message: %v", sendErr)
						}

						// 返回清除序列，清除容器输入缓冲区，但不发送换行符
						return len(clearSequence), nil
					}

					// 如果缓冲区太小，直接阻止
					t.lastCommand = command
					t.commandBlocked = true
					t.commandBuffer = ""

					// 同步发送错误消息给前端
					errorMsg, _ := json.Marshal(TerminalMessage{
						Op:   "stdout",
						Data: "\r\n\x1b[31m[命令被阻止]\x1b[0m Command Permission Denied: " + err.Error() + "\r\n",
					})
					if sendErr := t.sockJSSession.Send(string(errorMsg)); sendErr != nil {
						klog.Errorf("Failed to send error message: %v", sendErr)
					}

					// 返回空数据，阻止换行符发送
					return 0, nil
				}

				// 命令通过检查
				klog.Infof("Terminal command allowed: '%s' for user %s", command, t.user.Name)
				t.lastCommand = command
				t.commandBlocked = false
				t.commandBuffer = ""
				// 只发送换行符，不重复发送命令（命令字符已经在输入时发送过了）
				return copy(p, msg.Data), nil
			} else {
				// 命令还在输入中，累积到缓冲区并实时发送数据让用户看到输入
				t.commandBuffer += msg.Data
				klog.V(3).Infof("Terminal Read stdin: user=%s, buffer='%s', new data='%s'",
					t.user.Name, t.commandBuffer, strings.ReplaceAll(msg.Data, "\n", "\\n"))
				// 实时发送数据，让用户看到输入
				return copy(p, msg.Data), nil
			}
		} else if t.user == nil {
			klog.Warningf("Terminal Read: user is nil, skipping command check (sessionId: %s)", t.id)
		} else {
			klog.V(3).Infof("Terminal Read: admin user %s, skipping command check", t.user.Name)
		}
		// 将客户端输入数据复制到 p 缓冲区并返回
		return copy(p, msg.Data), nil
	case "resize":
		// 将终端尺寸变化发送到 sizeChan
		t.sizeChan <- remotecommand.TerminalSize{Width: msg.Cols, Height: msg.Rows}
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown message type '%s'", msg.Op)
	}
}

// 将数据（如容器输出）写入 WebSocket 会话，发送给客户端
func (t *TerminalSession) Write(p []byte) (int, error) {
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
func (t *TerminalSession) Close(status uint32, reason string) {
	t.sockJSSession.Close(status, reason)
	klog.Infof("close socket (%s). %d, %s", t.id, status, reason)
}

// 预检测shell可用性，使用缓存优化性能
func preCheckShell(k8sClient *kubernetes.Clientset, cfg *rest.Config, namespace, pod, container string) (string, error) {
	cacheKey := fmt.Sprintf("%s-%s-%s", namespace, pod, container)

	// 检查缓存
	shellCacheMux.RLock()
	if cached, exists := shellCacheMap[cacheKey]; exists && time.Since(cached.timestamp) < shellCacheTTL {
		shellCacheMux.RUnlock()
		klog.V(2).Infof("Using cached shell for %s: %s", cacheKey, cached.shell)
		return cached.shell, nil
	}
	shellCacheMux.RUnlock()

	// 预检测可用的shell
	validShells := []string{"bash", "sh"}
	var detectedShell string

	for _, shell := range validShells {
		// 使用快速命令检测shell是否存在
		req := k8sClient.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(pod).
			Namespace(namespace).
			SubResource("exec")

		req.VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"which", shell},
			Stdin:     false,
			Stdout:    true,
			Stderr:    false,
			TTY:       false,
		}, scheme.ParameterCodec)

		exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
		if err != nil {
			continue
		}

		// 快速检测shell是否存在
		var stdout bytes.Buffer
		err = exec.Stream(remotecommand.StreamOptions{
			Stdout: &stdout,
		})

		if err == nil && stdout.String() != "" {
			detectedShell = shell
			break
		}
	}

	// 如果没有检测到，使用默认shell
	if detectedShell == "" {
		detectedShell = "sh"
	}

	// 更新缓存
	shellCacheMux.Lock()
	shellCacheMap[cacheKey] = &ShellCache{
		shell:     detectedShell,
		timestamp: time.Now(),
	}
	shellCacheMux.Unlock()

	klog.V(2).Infof("Detected shell for %s: %s", cacheKey, detectedShell)
	return detectedShell, nil
}

// checkCommandPermission 检查命令权限
func checkCommandPermission(user *models.User, command string) error {
	if user == nil {
		klog.Warningf("checkCommandPermission: user is nil, allowing command")
		return nil
	}

	if user.Admin {
		// 管理员无限制
		klog.V(3).Infof("checkCommandPermission: admin user, allowing command: %s", command)
		return nil
	}

	// 确定用户角色
	role := "user"
	if user.Admin {
		role = "admin"
	}

	klog.Infof("checkCommandPermission: checking command '%s' for user '%s' (role: %s)", command, user.Name, role)

	// 获取该角色的规则
	rules, err := models.GetEnabledRulesByRole(role)
	if err != nil {
		klog.Warningf("Failed to get command rules for role %s: %v", role, err)
		return nil // 如果获取规则失败，允许执行（fail-open）
	}

	// 如果没有规则，允许执行
	if len(rules) == 0 {
		klog.V(3).Infof("checkCommandPermission: no rules for role %s, allowing command", role)
		return nil
	}

	// 检查命令是否匹配规则
	command = strings.TrimSpace(command)
	if command == "" {
		return nil // 空命令允许
	}

	// 提取命令的第一个单词（命令名）
	commandParts := strings.Fields(command)
	if len(commandParts) == 0 {
		return nil
	}
	commandName := commandParts[0]

	klog.Infof("checkCommandPermission: command='%s', commandName='%s', total rules=%d", command, commandName, len(rules))

	// 分离黑名单和白名单规则
	var blacklistRules []models.TerminalCommandRule
	var whitelistRules []models.TerminalCommandRule

	for _, rule := range rules {
		if rule.RuleType == models.RuleTypeBlacklist {
			blacklistRules = append(blacklistRules, rule)
		} else {
			whitelistRules = append(whitelistRules, rule)
		}
	}

	klog.Infof("checkCommandPermission: blacklist rules=%d, whitelist rules=%d", len(blacklistRules), len(whitelistRules))

	// 如果有白名单规则，必须匹配白名单
	if len(whitelistRules) > 0 {
		matched := false
		for _, rule := range whitelistRules {
			if matchCommand(command, commandName, rule.Command) {
				klog.Infof("checkCommandPermission: command '%s' matched whitelist rule: %s", command, rule.Command)
				matched = true
				break
			}
		}
		if !matched {
			klog.Warningf("checkCommandPermission: command '%s' not in whitelist, blocking", command)
			return fmt.Errorf("command not in whitelist")
		}
	}

	// 检查黑名单
	for _, rule := range blacklistRules {
		if matchCommand(command, commandName, rule.Command) {
			klog.Warningf("checkCommandPermission: command '%s' matched blacklist rule: %s (desc: %s), blocking",
				command, rule.Command, rule.Description)
			return fmt.Errorf("command '%s' is blocked by rule: %s", command, rule.Description)
		}
	}

	klog.Infof("checkCommandPermission: command '%s' passed all checks, allowing", command)
	return nil
}

// matchCommand 检查命令是否匹配规则（支持正则表达式和逗号分隔的模式列表）
func matchCommand(fullCommand, commandName, pattern string) bool {
	klog.V(3).Infof("matchCommand: fullCommand='%s', commandName='%s', pattern='%s'", fullCommand, commandName, pattern)

	// 处理逗号分隔的模式列表（如 "echo,cat,tail,cd,ls"）
	patterns := strings.Split(pattern, ",")
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// 检查是否匹配命令名或完整命令
		if matchSinglePattern(fullCommand, commandName, p) {
			klog.V(3).Infof("matchCommand: matched pattern '%s'", p)
			return true
		}
	}
	klog.V(3).Infof("matchCommand: no match found")
	return false
}

// matchSinglePattern 检查命令是否匹配单个模式
func matchSinglePattern(fullCommand, commandName, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	klog.V(4).Infof("matchSinglePattern: fullCommand='%s', commandName='%s', pattern='%s'", fullCommand, commandName, pattern)

	// 如果模式包含正则表达式特殊字符，使用正则匹配
	hasRegexChars := strings.ContainsAny(pattern, "*^$[]()+?{}|\\")

	if hasRegexChars {
		// 尝试正则表达式匹配（匹配完整命令）
		matched, err := regexp.MatchString(pattern, fullCommand)
		if err == nil && matched {
			klog.V(4).Infof("matchSinglePattern: regex matched fullCommand")
			return true
		}
		// 也尝试匹配命令名
		matched, err = regexp.MatchString(pattern, commandName)
		if err == nil && matched {
			klog.V(4).Infof("matchSinglePattern: regex matched commandName")
			return true
		}
		if err != nil {
			klog.Warningf("matchSinglePattern: regex error: %v", err)
		}
	}

	// 精确匹配命令名（最优先）
	if commandName == pattern {
		klog.V(4).Infof("matchSinglePattern: exact match commandName")
		return true
	}

	// 检查命令名是否以模式开头（用于支持 "echo" 匹配 "echo hello"）
	if strings.HasPrefix(commandName, pattern) {
		klog.V(4).Infof("matchSinglePattern: commandName starts with pattern")
		return true
	}

	// 简单字符串包含匹配：检查命令名或完整命令是否包含模式
	if strings.Contains(commandName, pattern) || strings.Contains(fullCommand, pattern) {
		klog.V(4).Infof("matchSinglePattern: contains match")
		return true
	}

	klog.V(4).Infof("matchSinglePattern: no match")
	return false
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

	// 从 session 存储中获取用户信息
	var user *models.User
	sessionUserMux.RLock()
	if u, exists := sessionUserMap[tr.SessionId]; exists {
		user = u
		klog.Infof("handleTerminalSession: found user for sessionId %s: %s (admin: %v)", tr.SessionId, u.Name, u.Admin)
	} else {
		// 获取所有可用的 sessionId 用于调试
		keys := make([]string, 0, len(sessionUserMap))
		for k := range sessionUserMap {
			keys = append(keys, k)
		}
		sessionUserMux.RUnlock()
		klog.Warningf("handleTerminalSession: user not found for sessionId %s, available sessions: %v", tr.SessionId, keys)
		sessionUserMux.RLock()
	}
	sessionUserMux.RUnlock()

	manager, err := client.Manager(tr.Cluster)
	if err == nil {
		ts := &TerminalSession{
			id:             tr.SessionId,
			sockJSSession:  session,
			sizeChan:       make(chan remotecommand.TerminalSize, 10), // 增加缓冲区大小
			user:           user,
			commandBuffer:  "",
			lastCommand:    "",
			commandBlocked: false,
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

	// 获取用户信息
	userInterface, exists := c.Get("User")
	if exists {
		if user, ok := userInterface.(*models.User); ok {
			// 存储 sessionId -> user 的映射
			sessionUserMux.Lock()
			sessionUserMap[sessionId] = user
			sessionUserMux.Unlock()

			klog.Infof("Terminal: stored user for sessionId %s: %s (admin: %v)", sessionId, user.Name, user.Admin)

			// 设置过期清理
			go func(sid string) {
				time.Sleep(sessionUserTTL)
				sessionUserMux.Lock()
				delete(sessionUserMap, sid)
				sessionUserMux.Unlock()
				klog.V(2).Infof("Terminal: cleaned up session %s after TTL", sid)
			}(sessionId)
		} else {
			klog.Warningf("Terminal: user interface type assertion failed, type: %T", userInterface)
		}
	} else {
		klog.Warningf("Terminal: user not found in context")
	}

	klog.Infof("Terminal: creating session, sessionId: %s", sessionId)

	result := TerminalResult{
		SessionId: sessionId,
		Token:     generateToken(namespace, pod),
		Cluster:   cluster,
		Namespace: namespace,
		Pod:       pod,
		Container: container,
		Cmd:       cmd,
	}

	klog.Infof("Terminal: result: %+v", result)

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

// WaitForTerminal 等待并启动一个终端会话，使用预检测的shell优化性能
func WaitForTerminal(k8sClient *kubernetes.Clientset, cfg *rest.Config, ts *TerminalSession, namespace, pod, container, cmd string) {
	var err error

	if cmd != "" && isValidShell([]string{"bash", "sh"}, cmd) {
		// 使用指定的shell
		cmds := []string{cmd}
		err = startProcess(k8sClient, cfg, cmds, ts, namespace, pod, container)
	} else {
		// 使用预检测的shell，避免重复尝试
		shell, _ := preCheckShell(k8sClient, cfg, namespace, pod, container)
		cmds := []string{shell}
		err = startProcess(k8sClient, cfg, cmds, ts, namespace, pod, container)
	}

	if err != nil {
		// 启动失败，关闭并返回错误
		ts.Close(2, err.Error())
		return
	}

	// 启动成功，关闭终端并返回 "Process exited"
	ts.Close(1, "Process exited")
}

// 清理过期的shell缓存
func CleanupShellCache() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			shellCacheMux.Lock()
			now := time.Now()
			for key, cache := range shellCacheMap {
				if now.Sub(cache.timestamp) > shellCacheTTL {
					delete(shellCacheMap, key)
				}
			}
			shellCacheMux.Unlock()
		}
	}()
}
