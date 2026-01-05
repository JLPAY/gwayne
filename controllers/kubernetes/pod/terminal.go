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
	"github.com/hinshun/vt10x"
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
	initialized    bool                            // 标记终端是否已初始化
	sentBuffer     string                          // 已发送到容器的数据缓冲区，用于获取实际执行的命令
	vt             vt10x.Terminal                  // 虚拟终端状态机，用于获取实际执行的命令（包括 Tab 补全后的内容）
	vtMutex        sync.RWMutex                    // 保护虚拟终端状态的互斥锁
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
		// 所有用户（包括 admin）都需要进行命令检查
		// 但是 checkCommandPermission 函数会处理：
		// - admin 用户如果没有 admin 角色的规则，则允许所有命令
		// - 普通用户如果没有 user 角色的规则，则允许所有命令
		// - 如果有规则，则按照规则检查
		needCheck := true

		if needCheck {
			// 累积所有输入到缓冲区（用于调试）
			t.commandBuffer += msg.Data
			// 累积实际发送到容器的数据（包括 Tab 补全时的输入）
			t.sentBuffer += msg.Data

			// 检查是否输入了换行符（命令结束）
			hasNewline := strings.Contains(msg.Data, "\n") || strings.Contains(msg.Data, "\r")

			if hasNewline {
				// 先检查是否是空命令（只包含换行符和控制字符）
				cleanBuffer := cleanString(t.sentBuffer)
				trimmedBuffer := strings.TrimSpace(strings.TrimRight(cleanBuffer, "\n\r"))

				var command string

				// 如果 sentBuffer 只包含换行符或控制字符，认为是空命令
				if trimmedBuffer == "" {
					command = ""
					klog.V(3).Infof("Terminal Read: empty command detected (sentBuffer='%s')", t.sentBuffer)

					// 写入换行符到虚拟终端状态机
					t.vtMutex.Lock()
					if t.vt != nil {
						_, _ = t.vt.Write([]byte(msg.Data))
					}
					t.vtMutex.Unlock()
				} else {
					// 有实际输入，从虚拟终端获取完整命令（包括 Tab 补全后的内容）
					// 在写入换行符之前，先获取当前行的完整内容
					// 因为写入换行符后光标会移动到下一行
					t.vtMutex.RLock()
					if t.vt != nil {
						command = t.getCurrentCommandLine()
					}
					t.vtMutex.RUnlock()

					// 如果获取到的命令为空或看起来不对，尝试获取上一行
					if command == "" || (len(command) > 0 && (strings.Contains(command, "tt") || strings.Contains(command, "aa") || strings.Contains(command, "ii") || strings.Contains(command, "ll"))) {
						klog.Warningf("Terminal Read: command from current line looks wrong '%s', trying to get previous line", command)
						t.vtMutex.RLock()
						if t.vt != nil {
							prevCommand := t.getPreviousCommandLine()
							if prevCommand != "" {
								command = prevCommand
							}
						}
						t.vtMutex.RUnlock()
					}

					// 现在写入换行符到虚拟终端状态机
					t.vtMutex.Lock()
					if t.vt != nil {
						_, _ = t.vt.Write([]byte(msg.Data))
					}
					t.vtMutex.Unlock()
				}

				// 如果从虚拟终端获取的内容为空，回退到使用 sentBuffer
				if command == "" {
					cleanBuffer := cleanString(t.sentBuffer)
					command = strings.TrimSpace(strings.TrimRight(cleanBuffer, "\n\r"))
					klog.V(3).Infof("Terminal Read: fallback to sentBuffer: '%s'", command)
				}

				klog.Infof("Terminal Read: sentBuffer='%s' (len=%d), command from vt10x='%s'",
					t.sentBuffer, len(t.sentBuffer), command)

				userName := "unknown"
				if t.user != nil {
					userName = t.user.Name
				}

				// 再次清理命令，确保去除 prompt（因为 cleanPrompt 可能在 getCurrentCommandLine 中已经调用过，但这里再次确保）
				command = cleanPrompt(command)

				klog.Infof("Terminal command detected (after cleanPrompt): '%s' (user: %s, admin: %v, sessionId: %s)",
					command, userName, t.user != nil && t.user.Admin, t.id)

				// 如果命令为空，允许通过（用于清屏等操作）
				if command == "" {
					t.commandBuffer = ""
					t.sentBuffer = ""
					t.commandBlocked = false
					t.lastCommand = ""
					// 只发送换行符，不重复发送命令
					return copy(p, msg.Data), nil
				}

				// 终端初始化命令：允许通过，不进行拦截检查
				// 前端会发送 "echo wayne-init" 来检测终端是否就绪
				if command == "echo wayne-init" || strings.HasPrefix(command, "echo wayne-init") {
					klog.V(3).Infof("Terminal initialization command detected: '%s', allowing without check", command)
					t.initialized = true
					t.lastCommand = command
					t.commandBlocked = false
					t.commandBuffer = ""
					t.sentBuffer = ""
					// 只发送换行符，不重复发送命令
					return copy(p, msg.Data), nil
				}

				// 处理历史命令：bash 历史命令以 ! 开头（如 !123, !!, !-1 等）
				// 历史命令会在 shell 内部展开，我们无法在 WebSocket 层面拦截实际命令
				// 对于历史命令，暂时允许执行（因为无法知道实际要执行的命令）
				if strings.HasPrefix(command, "!") {
					klog.V(3).Infof("Terminal command is a history command: '%s', allowing (cannot check actual command)", command)
					t.lastCommand = command
					t.commandBlocked = false
					t.commandBuffer = ""
					t.sentBuffer = ""
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
					t.sentBuffer = ""
					// 阻止发送换行符，防止命令执行
					return 0, nil
				}

				// 检查命令是否被禁止（user 为 nil 时，按普通用户处理）
				if err := checkCommandPermission(t.user, command); err != nil {
					klog.Warningf("Command blocked: '%s' for user %s, reason: %v", command, userName, err)

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
						t.sentBuffer = ""

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
					t.sentBuffer = ""

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
				klog.Infof("Terminal command allowed: '%s' for user %s", command, userName)
				t.lastCommand = command
				t.commandBlocked = false
				// 清空缓冲区
				t.commandBuffer = ""
				t.sentBuffer = ""
				// 发送换行符（命令字符已经在输入时发送过了）
				return copy(p, msg.Data), nil
			} else {
				// 命令还在输入中，实时发送数据让用户看到输入（包括 Tab 补全的效果）
				userName := "unknown"
				if t.user != nil {
					userName = t.user.Name
				}
				klog.V(3).Infof("Terminal Read stdin: user=%s, sentBuffer='%s' (len=%d), new data='%s'",
					userName, t.sentBuffer, len(t.sentBuffer), strings.ReplaceAll(msg.Data, "\n", "\\n"))
				// 实时发送数据，让用户看到输入
				return copy(p, msg.Data), nil
			}
		} else {
			// 管理员用户，跳过命令检查
			klog.V(3).Infof("Terminal Read: admin user %s, skipping command check", t.user.Name)
		}
		// 将客户端输入数据复制到 p 缓冲区并返回
		return copy(p, msg.Data), nil
	case "resize":
		// 将终端尺寸变化发送到 sizeChan
		t.sizeChan <- remotecommand.TerminalSize{Width: msg.Cols, Height: msg.Rows}
		// 同步更新虚拟终端状态机的尺寸
		t.vtMutex.Lock()
		if t.vt != nil {
			t.vt.Resize(int(msg.Cols), int(msg.Rows))
		}
		t.vtMutex.Unlock()
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown message type '%s'", msg.Op)
	}
}

// 将数据（如容器输出）写入 WebSocket 会话，发送给客户端
func (t *TerminalSession) Write(p []byte) (int, error) {
	// 将 stdout 输出传递给虚拟终端状态机（用于获取 Tab 补全后的完整命令）
	t.vtMutex.Lock()
	if t.vt != nil {
		_, _ = t.vt.Write(p)
	}
	t.vtMutex.Unlock()

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

// parseCommands 解析命令，提取所有子命令（处理管道、&&、|| 等）
// 例如："tail -100f | grep test" -> ["tail -100f", "grep test"]
// 例如："ls && cat file" -> ["ls", "cat file"]
func parseCommands(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	var commands []string
	var current strings.Builder
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(command); i++ {
		char := command[i]

		// 处理引号
		if char == '"' || char == '\'' {
			if !inQuotes {
				inQuotes = true
				quoteChar = char
			} else if char == quoteChar {
				inQuotes = false
				quoteChar = 0
			}
			current.WriteByte(char)
			continue
		}

		// 在引号内，直接添加字符
		if inQuotes {
			current.WriteByte(char)
			continue
		}

		// 检查管道符、&&、||
		if char == '|' {
			// 检查是否是 ||（逻辑或）
			if i+1 < len(command) && command[i+1] == '|' {
				// 保存当前命令
				cmd := strings.TrimSpace(current.String())
				if cmd != "" {
					commands = append(commands, cmd)
				}
				current.Reset()
				i++ // 跳过下一个 |
				continue
			}
			// 普通管道符 |
			cmd := strings.TrimSpace(current.String())
			if cmd != "" {
				commands = append(commands, cmd)
			}
			current.Reset()
			continue
		}

		if char == '&' {
			// 检查是否是 &&（逻辑与）
			if i+1 < len(command) && command[i+1] == '&' {
				cmd := strings.TrimSpace(current.String())
				if cmd != "" {
					commands = append(commands, cmd)
				}
				current.Reset()
				i++ // 跳过下一个 &
				continue
			}
			// 单个 &（后台运行），作为命令分隔符
			cmd := strings.TrimSpace(current.String())
			if cmd != "" {
				commands = append(commands, cmd)
			}
			current.Reset()
			continue
		}

		// 其他字符直接添加
		current.WriteByte(char)
	}

	// 添加最后一个命令
	cmd := strings.TrimSpace(current.String())
	if cmd != "" {
		commands = append(commands, cmd)
	}

	return commands
}

// getCurrentCommandLine 从虚拟终端状态机中获取当前光标所在行的完整命令（包括 Tab 补全后的内容）
func (t *TerminalSession) getCurrentCommandLine() string {
	if t.vt == nil {
		return ""
	}

	rows, cols := t.vt.Size()
	cursor := t.vt.Cursor()

	// 获取当前光标所在的行号
	cursorRow := cursor.Y
	if cursorRow < 0 || cursorRow >= rows {
		klog.V(3).Infof("getCurrentCommandLine: cursor row %d out of range [0, %d)", cursorRow, rows)
		return ""
	}

	// 遍历这一行的所有列，提取字符
	var line strings.Builder
	for j := 0; j < cols; j++ {
		cell := t.vt.Cell(j, cursorRow)
		if cell.Char != 0 {
			line.WriteRune(cell.Char)
		}
	}

	// 清理掉行末的空格和可能的 Prompt（如 "$" 或 "#"）
	fullLine := strings.TrimSpace(line.String())
	command := cleanPrompt(fullLine)

	klog.V(3).Infof("getCurrentCommandLine: cursorRow=%d, fullLine='%s', command='%s'", cursorRow, fullLine, command)
	return command
}

// getPreviousCommandLine 从虚拟终端状态机中获取上一行的完整命令（用于处理换行后的情况）
func (t *TerminalSession) getPreviousCommandLine() string {
	if t.vt == nil {
		return ""
	}

	rows, cols := t.vt.Size()
	cursor := t.vt.Cursor()

	// 获取上一行的行号（当前行 - 1）
	cursorRow := cursor.Y - 1
	if cursorRow < 0 || cursorRow >= rows {
		klog.V(3).Infof("getPreviousCommandLine: previous row %d out of range [0, %d)", cursorRow, rows)
		return ""
	}

	// 遍历上一行的所有列，提取字符
	var line strings.Builder
	for j := 0; j < cols; j++ {
		cell := t.vt.Cell(j, cursorRow)
		if cell.Char != 0 {
			line.WriteRune(cell.Char)
		}
	}

	// 清理掉行末的空格和可能的 Prompt（如 "$" 或 "#"）
	fullLine := strings.TrimSpace(line.String())
	command := cleanPrompt(fullLine)

	klog.Infof("getPreviousCommandLine: previousRow=%d, cols=%d, fullLine='%s', command='%s'",
		cursorRow, cols, fullLine, command)
	return command
}

// cleanPrompt 清理命令中的 Prompt 前缀（如 "root@pod:/# " 等）
func cleanPrompt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// 常见的 Prompt 格式：
	// - root@pod:/# command
	// - user@host:~/path$ command
	// - [root@pod /]# command
	// - bash-4.2$ command
	// - bash-4.2$ (空命令，只有 prompt)

	// 先尝试通过 # 分割
	parts := strings.Split(raw, "#")
	if len(parts) > 1 {
		// 取最后一部分，并清理空格
		command := strings.TrimSpace(parts[len(parts)-1])
		// 如果命令不为空，且不等于原始字符串（说明确实有命令），返回命令
		if command != "" && command != raw {
			return command
		}
	}

	// 再尝试通过 $ 分割
	parts = strings.Split(raw, "$")
	if len(parts) > 1 {
		// 取最后一部分，并清理空格
		command := strings.TrimSpace(parts[len(parts)-1])
		// 如果命令不为空，且不等于原始字符串（说明确实有命令），返回命令
		if command != "" && command != raw {
			return command
		}
	}

	// 如果都没有找到，尝试通过正则表达式匹配常见的 Prompt 模式
	// 例如：[root@pod /]# 或 user@host:~/path$ 或 bash-4.2$
	promptPattern := regexp.MustCompile(`^[^\$#]*[\$#]\s*`)
	command := promptPattern.ReplaceAllString(raw, "")
	command = strings.TrimSpace(command)

	// 如果清理后为空，或者清理后的命令等于原始字符串（说明没有找到 prompt），返回空字符串
	// 因为如果原始字符串本身就是 prompt（如 "bash-4.2$"），清理后应该为空
	if command == "" || command == raw {
		// 检查原始字符串是否看起来像 prompt（包含 $ 或 # 且没有其他内容）
		if strings.HasSuffix(raw, "$") || strings.HasSuffix(raw, "#") {
			// 这看起来像是一个只有 prompt 的行，返回空字符串
			return ""
		}
		// 检查是否是控制字符显示（如 ^C, ^D, ^Z 等）
		// 这些是终端显示的控制字符，不是真正的命令
		controlCharPattern := regexp.MustCompile(`^\^[A-Z]$`)
		if controlCharPattern.MatchString(raw) {
			// 这是控制字符显示（如 ^C），返回空字符串
			return ""
		}
		// 如果原始字符串不包含 $ 或 #，可能是真正的命令
		return raw
	}

	// 检查清理后的命令是否是控制字符显示
	controlCharPattern := regexp.MustCompile(`^\^[A-Z]$`)
	if controlCharPattern.MatchString(command) {
		// 这是控制字符显示（如 ^C），返回空字符串
		return ""
	}

	return command
}

// cleanString 清理字符串，移除控制字符和不可见字符
func cleanString(s string) string {
	var result strings.Builder
	for _, r := range s {
		// 保留可打印字符和空格
		if r >= 32 && r < 127 || r == '\t' || r == '\n' || r == '\r' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// extractCommandName 提取命令名（第一个单词，忽略参数）
// 例如："tail -100f logs/file.log" -> "tail"
// 例如："ls -la | grep test" -> 先解析管道，再提取每个命令名
func extractCommandName(cmd string) string {
	originalCmd := cmd
	// 先清理控制字符和不可见字符
	cmd = cleanString(cmd)
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		klog.V(3).Infof("extractCommandName: empty command after trim")
		return ""
	}

	klog.V(3).Infof("extractCommandName: original='%s' (len=%d, bytes=%v), after clean and trim='%s' (len=%d)",
		originalCmd, len(originalCmd), []byte(originalCmd), cmd, len(cmd))

	// 处理命令名中可能包含的特殊字符（如重定向符号）
	// 注意：重定向符号可能在命令名后面，所以要先提取第一个单词
	// 提取第一个单词（命令名）- 先提取，再处理特殊字符
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		klog.V(3).Infof("extractCommandName: no fields found in '%s'", cmd)
		return ""
	}

	commandName := strings.TrimSpace(parts[0])
	// 再次清理，确保没有控制字符
	commandName = cleanString(commandName)
	commandName = strings.TrimSpace(commandName)

	klog.Infof("extractCommandName: extracted commandName='%s' (len=%d, bytes=%v) from cmd='%s'",
		commandName, len(commandName), []byte(commandName), cmd)

	// 处理命令名中可能包含的特殊字符（如重定向符号）
	if idx := strings.IndexAny(commandName, "<>;"); idx > 0 {
		commandName = commandName[:idx]
		klog.V(3).Infof("extractCommandName: after removing special chars, commandName='%s'", commandName)
	}

	return commandName
}

// checkCommandPermission 检查命令权限
func checkCommandPermission(user *models.User, command string) error {
	// 确定用户角色：user 为 nil 时，按普通用户处理
	role := "user"
	userName := "unknown"
	isAdmin := false

	if user != nil {
		userName = user.Name
		isAdmin = user.Admin
		if isAdmin {
			role = "admin"
		} else {
			role = "user"
		}
	}

	klog.Infof("checkCommandPermission: checking command '%s' for user '%s' (role: %s, isAdmin: %v)", command, userName, role, isAdmin)

	// 检查命令是否匹配规则
	command = strings.TrimSpace(command)
	if command == "" {
		return nil // 空命令允许
	}

	// 处理历史命令：bash 历史命令以 ! 开头（如 !123, !!, !-1 等）
	// 历史命令会在 shell 内部展开，我们无法在 WebSocket 层面拦截实际命令
	// 对于历史命令，暂时允许执行（因为无法知道实际要执行的命令）
	if strings.HasPrefix(command, "!") {
		klog.V(3).Infof("checkCommandPermission: history command '%s', allowing (cannot check actual command)", command)
		return nil
	}

	// 获取该角色的规则
	rules, err := models.GetEnabledRulesByRole(role)
	if err != nil {
		klog.Warningf("Failed to get command rules for role %s: %v", role, err)
		// 如果获取规则失败，对于 admin 用户允许执行（fail-open），对于普通用户也允许执行
		return nil
	}

	// 如果没有规则，允许执行
	// 特别地，对于 admin 用户，如果没有设置 admin 角色的规则，则完全不受限制
	if len(rules) == 0 {
		if isAdmin {
			klog.V(3).Infof("checkCommandPermission: admin user '%s' has no rules configured, allowing all commands", userName)
		} else {
			klog.V(3).Infof("checkCommandPermission: no rules for role %s, allowing command", role)
		}
		return nil
	}

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
	klog.Infof("checkCommandPermission: original command string='%s' (len=%d)", command, len(command))

	// 第一步：解析命令，提取所有子命令（处理管道、&&、|| 等）
	commands := parseCommands(command)
	if len(commands) == 0 {
		klog.Warningf("checkCommandPermission: no commands parsed from '%s'", command)
		return nil
	}

	klog.Infof("checkCommandPermission: command='%s', parsed commands=%v (count=%d)", command, commands, len(commands))

	// 第二步：对每个命令，提取命令名并检查黑白名单
	for i, cmd := range commands {
		klog.Infof("checkCommandPermission: processing command[%d]='%s' (len=%d)", i, cmd, len(cmd))

		// 提取命令名（忽略参数）
		commandName := extractCommandName(cmd)
		if commandName == "" {
			klog.Warningf("checkCommandPermission: failed to extract commandName from '%s'", cmd)
			continue
		}

		klog.Infof("checkCommandPermission: checking command '%s' (commandName='%s')", cmd, commandName)

		// 如果有白名单规则，每个命令名都必须在白名单中
		if len(whitelistRules) > 0 {
			matched := false
			for _, rule := range whitelistRules {
				klog.Infof("checkCommandPermission: checking commandName '%s' against whitelist rule: '%s'", commandName, rule.Command)
				if matchCommandName(commandName, rule.Command) {
					klog.Infof("checkCommandPermission: commandName '%s' matched whitelist rule: %s", commandName, rule.Command)
					matched = true
					break
				} else {
					klog.V(3).Infof("checkCommandPermission: commandName '%s' did not match whitelist rule: %s", commandName, rule.Command)
				}
			}
			if !matched {
				klog.Warningf("checkCommandPermission: commandName '%s' not in whitelist, blocking. Total whitelist rules: %d", commandName, len(whitelistRules))
				return fmt.Errorf("command '%s' not in whitelist", commandName)
			}
		}

		// 检查黑名单：如果任何一个命令名匹配黑名单，就阻止
		for _, rule := range blacklistRules {
			if matchCommandName(commandName, rule.Command) {
				klog.Warningf("checkCommandPermission: commandName '%s' matched blacklist rule: %s (desc: %s), blocking",
					commandName, rule.Command, rule.Description)
				return fmt.Errorf("command '%s' is blocked by rule: %s", commandName, rule.Description)
			}
		}
	}

	klog.Infof("checkCommandPermission: command '%s' passed all checks, allowing", command)
	return nil
}

// matchCommandName 检查命令名是否匹配规则（只匹配命令名，不匹配参数）
// pattern 可以是逗号分隔的列表，如 "echo,cat,tail,cd,ls"
// 只支持精确匹配（全匹配），不支持正则表达式
func matchCommandName(commandName, pattern string) bool {
	commandName = strings.TrimSpace(commandName)
	pattern = strings.TrimSpace(pattern)

	if commandName == "" || pattern == "" {
		klog.V(3).Infof("matchCommandName: empty commandName or pattern, commandName='%s', pattern='%s'", commandName, pattern)
		return false
	}

	klog.Infof("matchCommandName: commandName='%s', pattern='%s'", commandName, pattern)

	// 处理逗号分隔的模式列表（如 "echo,cat,tail,cd,ls"）
	patterns := strings.Split(pattern, ",")
	klog.Infof("matchCommandName: split patterns: %v (count=%d)", patterns, len(patterns))

	for i, p := range patterns {
		originalP := p
		p = strings.TrimSpace(p)
		if p == "" {
			klog.Infof("matchCommandName: skipping empty pattern[%d]='%s'", i, originalP)
			continue
		}

		klog.Infof("matchCommandName: checking commandName '%s' (len=%d) against pattern[%d]='%s' (len=%d, original='%s')",
			commandName, len(commandName), i, p, len(p), originalP)

		matchResult := matchSingleCommandName(commandName, p)
		klog.Infof("matchCommandName: matchSingleCommandName returned %v for commandName='%s' pattern='%s'",
			matchResult, commandName, p)

		if matchResult {
			klog.Infof("matchCommandName: commandName '%s' matched pattern '%s'", commandName, p)
			return true
		} else {
			klog.Infof("matchCommandName: commandName '%s' did NOT match pattern '%s'", commandName, p)
		}
	}

	klog.Infof("matchCommandName: commandName '%s' no match found in pattern '%s'", commandName, pattern)
	return false
}

// matchSingleCommandName 检查命令名是否匹配单个模式（只支持精确匹配，不支持正则表达式）
func matchSingleCommandName(commandName, pattern string) bool {
	// 对两者都进行 trim，确保没有前导或尾随空格
	commandName = strings.TrimSpace(commandName)
	pattern = strings.TrimSpace(pattern)

	if pattern == "" {
		klog.V(3).Infof("matchSingleCommandName: empty pattern")
		return false
	}

	if commandName == "" {
		klog.V(3).Infof("matchSingleCommandName: empty commandName")
		return false
	}

	klog.Infof("matchSingleCommandName: commandName='%s' (len=%d), pattern='%s' (len=%d)",
		commandName, len(commandName), pattern, len(pattern))

	// 只支持精确匹配（全匹配）
	if commandName == pattern {
		klog.Infof("matchSingleCommandName: exact match - commandName='%s' == pattern='%s'", commandName, pattern)
		return true
	}

	// 如果不匹配，输出详细的比较信息
	klog.Infof("matchSingleCommandName: no match - commandName='%s' (len=%d, bytes=%v) != pattern='%s' (len=%d, bytes=%v)",
		commandName, len(commandName), []byte(commandName), pattern, len(pattern), []byte(pattern))
	return false
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
		// 正则表达式匹配失败，继续下面的精确匹配
	}

	// 精确匹配命令名（最优先，用于白名单和黑名单）
	// 例如：pattern="tail" 应该匹配 commandName="tail"，无论参数是什么
	if commandName == pattern {
		klog.V(4).Infof("matchSinglePattern: exact match commandName")
		return true
	}

	// 对于非正则表达式模式，不再使用包含匹配，避免误匹配
	// 例如：pattern="ai" 不应该匹配 commandName="tail"
	// 例如：pattern="100" 不应该匹配 fullCommand="tail -100f"

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
		// 创建虚拟终端状态机（默认大小 80x24，会在 resize 时更新）
		vt := vt10x.New()
		if vt != nil {
			vt.Resize(80, 24) // 默认大小
		}

		ts := &TerminalSession{
			id:             tr.SessionId,
			sockJSSession:  session,
			sizeChan:       make(chan remotecommand.TerminalSize, 10), // 增加缓冲区大小
			user:           user,
			commandBuffer:  "",
			lastCommand:    "",
			commandBlocked: false,
			initialized:    false,
			sentBuffer:     "",
			vt:             vt,
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
