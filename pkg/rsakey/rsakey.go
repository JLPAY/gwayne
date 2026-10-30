package rsakey

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"k8s.io/klog/v2"
)

var (
	RsaPrivateKey *rsa.PrivateKey
	RsaPublicKey  *rsa.PublicKey
	// RsaKid 是当前签发密钥的公钥指纹，写入 JWT kid 头
	RsaKid string

	// db 模式下的旧密钥（previous）：轮换宽限窗口内仅用于验签，不再用于签发
	prevKid       string
	prevPublicKey *rsa.PublicKey
	prevExpireAt  time.Time

	// keyMux 保护以上密钥状态（db 模式下可能被惰性同步并发更新）
	keyMux sync.RWMutex

	// syncThrottle 惰性同步节流：收到未知 kid 时最多每 minSyncInterval 从库同步一次
	syncThrottle = struct {
		sync.Mutex
		at time.Time
	}{}
)

// JwtIssuer 是服务端签发令牌时使用的 iss 声明，所有校验端必须一致。
const JwtIssuer = "gwayne"

// compromisedDefaultPubPEM_SHA256 是 Qihoo360/wayne 开源仓库自带的默认 RSA 公钥
// （conf/rsa-public.pem / apikey/rsa-public.pem 的初始内容）的文件级 SHA256 指纹。
const compromisedDefaultPubPEM_SHA256 = "2aa25af7280670152a45a9db8042a2d627201e8a5c7a45109cd223daf675c33c"

// compromisedDefaultPubDER_SHA256 是同一默认公钥 DER 编码的 SHA256 指纹，
// 用于 DB 密钥模式：即使入库时 PEM 被重编码，仍可识别出默认密钥。
const compromisedDefaultPubDER_SHA256 = "9022a61a223fd2c0abcfa2aa3bc5cf61232225df3381e10cf6677045977ce362"

const minSyncInterval = 10 * time.Second

// IsDBMode 是否启用 DB 密钥模式
func IsDBMode() bool {
	return strings.EqualFold(strings.TrimSpace(config.Conf.App.RsaKeyStore), "db")
}

// graceWindow previous 密钥的验签宽限窗口：≥ 最长令牌有效期，保证轮换不影响存量合法令牌
func graceWindow() time.Duration {
	sec := config.Conf.App.TokenLifeTime
	if sec <= 0 {
		sec = 86400
	}
	return time.Duration(sec) * time.Second
}

func InitRsaKey() {
	if IsDBMode() {
		initRsaKeyFromDB()
		return
	}
	initRsaKeyFromFiles()
}

// initRsaKeyFromFiles 文件密钥模式（默认）：读 conf 下密钥文件，缺失则本地生成
func initRsaKeyFromFiles() {
	var err error

	privPath := config.Conf.App.RsaPrivateKey
	pubPath := config.Conf.App.RsaPublicKey

	// 防御：若当前公钥与开源默认公钥逐字节一致，说明仍在使用已被公开的密钥，
	// 存在越权风险，强制删除并重新生成密钥对。
	if isCompromisedDefaultKeyFile(pubPath) {
		klog.Warningf("检测到使用开源仓库默认 RSA 公钥(%s)，存在越权伪造风险，强制重新生成密钥对", pubPath)
		_ = os.Remove(privPath)
		_ = os.Remove(pubPath)
	}

	// 检查密钥对是否存在，不存在生成密钥对
	if !keyExists(privPath) || !keyExists(pubPath) {
		klog.Info("rsa密钥对不存在，生成新的密钥对...")
		if err := generateRsaKeyPair(2048, filepath.Dir(privPath)); err != nil {
			klog.Exitf("生成密钥对失败 %s", err)
		}
	}

	RsaPrivateKey, err = jwt.ParseRSAPrivateKeyFromPEM(readKey(privPath))
	if err != nil {
		klog.Exitf("载入密钥: %s 失败 %s", privPath, err)
	}
	klog.Infof("载入密钥: %s 完成", privPath)

	RsaPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(readKey(pubPath))
	if err != nil {
		klog.Exitf("载入公钥: %s 失败 %s", pubPath, err)
	}
	klog.Infof("载入公钥: %s 完成", pubPath)

	keyMux.Lock()
	RsaKid = computeKid(RsaPublicKey)
	prevKid, prevPublicKey, prevExpireAt = "", nil, time.Time{}
	keyMux.Unlock()
	klog.Infof("当前 RSA 公钥指纹(kid)=%s（file 模式）", RsaKid)

	// 恢复上一轮轮换遗留的 previous 密钥（若存在且仍在宽限窗口内），
	// 保证「轮换 → 重启」后窗口内存量令牌依然可验签
	loadPreviousFromFiles()
}

// initRsaKeyFromDB DB 密钥模式：多副本共享同一行，保证令牌跨副本可验证。
// 首启自动生成唯一密钥入库；检测到开源默认密钥时强制轮换（自愈）。
func initRsaKeyFromDB() {
	for attempt := 0; attempt < 3; attempt++ {
		current, previous, err := models.LoadRsaKeys(graceWindow())
		if err != nil {
			klog.Exitf("DB 密钥模式: 读取密钥失败 %s", err)
		}

		// 首启播种：库中无密钥，生成唯一密钥写入（所有副本共享，签发/验签一致）
		if current == nil {
			klog.Info("DB 密钥模式: 库中无密钥，生成新的 RSA 密钥对...")
			if _, err := rotateInDB("初始化播种"); err != nil {
				klog.Errorf("DB 密钥模式: 初始化播种失败(第%d次) %s", attempt+1, err)
				continue
			}
			continue // 重读
		}

		// 默认密钥守卫：库中 current 为开源默认公钥 → 强制轮换
		if isCompromisedPublicKeyPEM([]byte(current.PublicKey)) {
			klog.Warningf("DB 密钥模式: 检测到开源默认 RSA 公钥(kid=%s)，已被公开可伪造任意管理员令牌，强制轮换", current.Kid)
			if _, err := rotateInDB("默认密钥守卫触发"); err != nil {
				klog.Errorf("DB 密钥模式: 守卫轮换失败(第%d次) %s", attempt+1, err)
				continue
			}
			continue // 重读
		}

		applyKeys(current, previous)
		return
	}
	klog.Exitf("DB 密钥模式: 初始化失败（连续多次播种/读取异常）")
}

// applyKeys 将库中的 current/previous 应用到内存
func applyKeys(current, previous *models.RsaKey) {
	priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(current.PrivateKey))
	if err != nil {
		klog.Exitf("DB 密钥模式: 解析私钥失败(kid=%s) %s", current.Kid, err)
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM([]byte(current.PublicKey))
	if err != nil {
		klog.Exitf("DB 密钥模式: 解析公钥失败(kid=%s) %s", current.Kid, err)
	}
	kid := computeKid(pub)
	if kid != current.Kid {
		// kid 与公钥不匹配，说明库中数据被篡改或异常，拒绝启动
		klog.Exitf("DB 密钥模式: 库中密钥 kid(%s) 与公钥指纹(%s) 不一致，疑似数据异常", current.Kid, kid)
	}

	keyMux.Lock()
	RsaPrivateKey, RsaPublicKey, RsaKid = priv, pub, kid
	prevKid, prevPublicKey, prevExpireAt = "", nil, time.Time{}
	if previous != nil {
		prevPub, err := jwt.ParseRSAPublicKeyFromPEM([]byte(previous.PublicKey))
		if err != nil {
			klog.Warningf("DB 密钥模式: 解析 previous 公钥失败(kid=%s)，忽略 %s", previous.Kid, err)
		} else if computeKid(prevPub) != previous.Kid {
			klog.Warningf("DB 密钥模式: previous kid(%s) 与公钥指纹不一致，忽略", previous.Kid)
		} else {
			prevKid = previous.Kid
			prevPublicKey = prevPub
			if previous.RotateTime != nil {
				prevExpireAt = previous.RotateTime.Add(graceWindow())
			}
		}
	}
	keyMux.Unlock()

	klog.Infof("DB 密钥模式: 当前签名密钥 kid=%s", kid)
	if prevKid != "" {
		klog.Infof("DB 密钥模式: previous 密钥 kid=%s 宽限至 %s", prevKid, prevExpireAt.Format("2006-01-02 15:04:05"))
	}
}

// PublicKeyForKid 按 JWT 的 kid 头选择验签公钥：
// current 恒可用；previous 仅在宽限窗口内可用。未知 kid 返回 false。
func PublicKeyForKid(kid string) (*rsa.PublicKey, bool) {
	keyMux.RLock()
	defer keyMux.RUnlock()
	if kid != "" && kid == RsaKid {
		return RsaPublicKey, true
	}
	if kid != "" && kid == prevKid && prevPublicKey != nil && time.Now().Before(prevExpireAt) {
		return prevPublicKey, true
	}
	return nil, false
}

// CurrentKid 返回当前签发密钥的 kid
func CurrentKid() string {
	keyMux.RLock()
	defer keyMux.RUnlock()
	return RsaKid
}

// KeyInfo 密钥状态（不含任何私钥材料），供管理端展示
type KeyInfo struct {
	Mode             string `json:"mode"`
	CurrentKid       string `json:"currentKid"`
	PreviousKid      string `json:"previousKid,omitempty"`
	PreviousExpireAt string `json:"previousExpireAt,omitempty"`
}

func GetRsaKeyInfo() KeyInfo {
	keyMux.RLock()
	defer keyMux.RUnlock()
	info := KeyInfo{
		Mode:       map[bool]string{true: "db", false: "file"}[IsDBMode()],
		CurrentKid: RsaKid,
	}
	if prevKid != "" {
		info.PreviousKid = prevKid
		info.PreviousExpireAt = prevExpireAt.Format("2006-01-02 15:04:05")
	}
	return info
}

// Rotate 生成新密钥并轮换：
// 旧 current 降级为 previous，宽限窗口（≥ TokenLifeTime）内存量令牌仍可验签。
// db 模式：事务化写库，其他副本通过"未知 kid 触发惰性同步"在数秒内自动感知，无需重启。
// file 模式：新密钥原子落盘，previous 密钥持久化到 *-previous.pem（重启后宽限窗口依然有效）；
// 注意多副本部署时各副本密钥文件独立，轮换仅对当前实例生效（多副本请使用 db 模式）。
func Rotate(reason string) (KeyInfo, error) {
	if IsDBMode() {
		if _, err := rotateInDB(reason); err != nil {
			return KeyInfo{}, err
		}
		// 本副本立即应用新密钥（其他副本靠惰性同步感知）
		if err := syncFromDB(); err != nil {
			return KeyInfo{}, err
		}
		return GetRsaKeyInfo(), nil
	}
	if err := rotateInFiles(reason); err != nil {
		return KeyInfo{}, err
	}
	return GetRsaKeyInfo(), nil
}

// rotateMux 串行化文件模式轮换，避免并发轮换交错写文件
var rotateMux sync.Mutex

// rotateMeta previous 密钥的持久化元数据（file 模式）
type rotateMeta struct {
	PreviousKid string `json:"previous_kid"`
	RotatedAt   string `json:"rotated_at"` // RFC3339
}

// previousKeyPaths 返回 file 模式下 previous 私钥/公钥/元数据的落盘路径
// （与当前密钥同目录，命名 *-previous.pem / rsa-previous-meta.json）
func previousKeyPaths() (privPrev, pubPrev, meta string) {
	privPath := config.Conf.App.RsaPrivateKey
	pubPath := config.Conf.App.RsaPublicKey
	privPrev = withPemSuffix(privPath, "-previous.pem")
	pubPrev = withPemSuffix(pubPath, "-previous.pem")
	meta = filepath.Join(filepath.Dir(privPath), "rsa-previous-meta.json")
	return
}

func withPemSuffix(p, suffix string) string {
	if strings.HasSuffix(p, ".pem") {
		return strings.TrimSuffix(p, ".pem") + suffix
	}
	return p + suffix
}

// rotateInFiles file 密钥模式在线轮换：
// 1) 当前密钥降级为 previous 并持久化（重启后宽限窗口依然有效）
// 2) 新密钥原子落盘（临时文件+rename，避免中断导致半写状态）
// 3) 内存立即切换，新令牌即时用新密钥签发
func rotateInFiles(reason string) error {
	rotateMux.Lock()
	defer rotateMux.Unlock()

	privPath := config.Conf.App.RsaPrivateKey
	pubPath := config.Conf.App.RsaPublicKey

	// 序列化当前密钥（轮换前的 current 即新的 previous）
	keyMux.RLock()
	curKid := RsaKid
	curPrivPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(RsaPrivateKey),
	})
	curPubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mustMarshalPKIX(RsaPublicKey),
	})
	keyMux.RUnlock()

	// 生成新密钥对
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成 RSA 密钥失败: %w", err)
	}
	newPrivPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	newPubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mustMarshalPKIX(&priv.PublicKey),
	})

	// 先持久化 previous，再落新密钥：任一步失败都不影响当前内存中的有效状态
	prevPrivPath, prevPubPath, metaPath := previousKeyPaths()
	if err := writeFileAtomic(prevPrivPath, curPrivPEM, 0600); err != nil {
		return fmt.Errorf("持久化 previous 私钥失败: %w", err)
	}
	if err := writeFileAtomic(prevPubPath, curPubPEM, 0644); err != nil {
		return fmt.Errorf("持久化 previous 公钥失败: %w", err)
	}
	metaBytes, _ := json.Marshal(rotateMeta{PreviousKid: curKid, RotatedAt: time.Now().Format(time.RFC3339)})
	if err := writeFileAtomic(metaPath, metaBytes, 0644); err != nil {
		return fmt.Errorf("持久化轮换元数据失败: %w", err)
	}
	if err := writeFileAtomic(privPath, newPrivPEM, 0600); err != nil {
		return fmt.Errorf("写入新私钥失败: %w", err)
	}
	if err := writeFileAtomic(pubPath, newPubPEM, 0644); err != nil {
		return fmt.Errorf("写入新公钥失败: %w", err)
	}

	// 内存切换：旧 current → previous（宽限窗口），新密钥 → current
	newKid := computeKid(&priv.PublicKey)
	keyMux.Lock()
	prevKid, prevPublicKey, prevExpireAt = curKid, RsaPublicKey, time.Now().Add(graceWindow())
	RsaPrivateKey, RsaPublicKey, RsaKid = priv, &priv.PublicKey, newKid
	keyMux.Unlock()

	klog.Infof("RSA 密钥已轮换(file): 新 kid=%s 旧 kid=%s 宽限至 %s 原因=%s",
		newKid, curKid, time.Now().Add(graceWindow()).Format("2006-01-02 15:04:05"), reason)
	return nil
}

// loadPreviousFromFiles 启动时恢复 previous 密钥：
// 元数据缺失/损坏、已过宽限窗口、公钥异常或命中开源默认密钥时清理并忽略
func loadPreviousFromFiles() {
	prevPrivPath, prevPubPath, metaPath := previousKeyPaths()
	if !keyExists(prevPubPath) || !keyExists(metaPath) {
		return
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}
	var meta rotateMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil || meta.PreviousKid == "" {
		klog.Warningf("file 模式: 轮换元数据损坏，清理 previous 密钥文件")
		cleanupPreviousFiles(prevPrivPath, prevPubPath, metaPath)
		return
	}
	rotatedAt, err := time.Parse(time.RFC3339, meta.RotatedAt)
	if err != nil {
		cleanupPreviousFiles(prevPrivPath, prevPubPath, metaPath)
		return
	}
	expireAt := rotatedAt.Add(graceWindow())
	if !time.Now().Before(expireAt) {
		klog.Infof("file 模式: previous 密钥(kid=%s)已过宽限窗口，清理", meta.PreviousKid)
		cleanupPreviousFiles(prevPrivPath, prevPubPath, metaPath)
		return
	}
	pubPEM, err := os.ReadFile(prevPubPath)
	if err != nil {
		return
	}
	// 守卫：previous 也绝不允许是已泄露的开源默认公钥
	if isCompromisedPublicKeyPEM(pubPEM) {
		klog.Warningf("file 模式: previous 公钥为开源默认密钥，直接清理（不进入验签窗口）")
		cleanupPreviousFiles(prevPrivPath, prevPubPath, metaPath)
		return
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM(pubPEM)
	if err != nil || computeKid(pub) != meta.PreviousKid {
		klog.Warningf("file 模式: previous 公钥解析/指纹校验失败，清理")
		cleanupPreviousFiles(prevPrivPath, prevPubPath, metaPath)
		return
	}

	keyMux.Lock()
	prevKid, prevPublicKey, prevExpireAt = meta.PreviousKid, pub, expireAt
	keyMux.Unlock()
	klog.Infof("file 模式: 恢复 previous 密钥 kid=%s 宽限至 %s",
		meta.PreviousKid, expireAt.Format("2006-01-02 15:04:05"))
}

func cleanupPreviousFiles(paths ...string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// mustMarshalPKIX 序列化公钥为 DER（失败直接 panic：仅在内存密钥结构异常时发生）
func mustMarshalPKIX(pub *rsa.PublicKey) []byte {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		klog.Exitf("marshal public key failed: %s", err)
	}
	return der
}

// writeFileAtomic 原子写文件：同目录临时文件 + rename，避免中断产生半写文件
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rsa-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName) // rename 成功后不存在，失败时兜底清理
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// rotateInDB 生成新密钥对并调用 models 事务轮换
func rotateInDB(reason string) (*models.RsaKey, error) {
	generate := func() (kid, privatePEM, publicPEM string, err error) {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", "", "", fmt.Errorf("生成 RSA 密钥失败: %w", err)
		}
		privPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		})
		pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		if err != nil {
			return "", "", "", fmt.Errorf("序列化公钥失败: %w", err)
		}
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
		return computeKid(&priv.PublicKey), string(privPEM), string(pubPEM), nil
	}

	row, err := models.RotateRsaKey(generate)
	if err != nil {
		return nil, err
	}
	klog.Infof("RSA 密钥已轮换: 新 kid=%s 原因=%s", row.Kid, reason)
	return row, nil
}

// syncFromDB 从库重读密钥并应用（惰性同步：副本无需重启即可感知其他副本发起的轮换）
func syncFromDB() error {
	current, previous, err := models.LoadRsaKeys(graceWindow())
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("库中无 current 密钥")
	}
	applyKeys(current, previous)
	return nil
}

// SyncFromDBIfStale 收到未知 kid 时的节流同步：最多每 minSyncInterval 尝试一次。
// 返回 true 表示同步后密钥状态已更新（调用方可重试验证）。
func SyncFromDBIfStale() bool {
	if !IsDBMode() {
		return false
	}
	syncThrottle.Lock()
	if time.Since(syncThrottle.at) < minSyncInterval {
		syncThrottle.Unlock()
		return false
	}
	syncThrottle.at = time.Now()
	syncThrottle.Unlock()

	if err := syncFromDB(); err != nil {
		klog.Warningf("惰性同步密钥失败: %s", err)
		return false
	}
	return true
}

// computeKid 基于公钥 DER 计算确定性指纹，作为 JWT kid。
func computeKid(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		klog.Exitf("marshal public key failed: %s", err)
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// isCompromisedPublicKeyPEM 判断 PEM 公钥是否为开源默认公钥（按 DER 指纹比对，兼容重编码）
func isCompromisedPublicKeyPEM(pemBytes []byte) bool {
	pub, err := jwt.ParseRSAPublicKeyFromPEM(pemBytes)
	if err != nil {
		return false
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]) == compromisedDefaultPubDER_SHA256
}

// isCompromisedDefaultKeyFile 判断公钥文件是否为开源默认公钥（按文件内容 SHA256 比对）
func isCompromisedDefaultKeyFile(pubPath string) bool {
	data, err := os.ReadFile(pubPath)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == compromisedDefaultPubPEM_SHA256
}

// 检查密钥文件是否存在
func keyExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// 生成 RSA 密钥对（文件模式）
func generateRsaKeyPair(bits int, path string) error {
	// 检查路径是否存在，如果不存在则创建
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, 0755) // 创建路径
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}

	// 生成RSA私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		klog.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyPath := filepath.Join(path, "rsa-private.pem")
	privFile, err := os.Create(privateKeyPath)
	if err != nil {
		klog.Fatalf("failed to create private key file: %v", err)
	}
	defer privFile.Close()

	privPem := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	// 写入文件
	err = pem.Encode(privFile, privPem)
	if err != nil {
		return fmt.Errorf("failed to write private key to file: %v", err)
	}
	klog.Info("Private key saved to", privateKeyPath)

	// 从私钥提取公钥
	pubKey := &privateKey.PublicKey

	publicKeyPath := filepath.Join(path, "rsa-public.pem")
	pubFile, err := os.Create(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %v", err)
	}
	defer pubFile.Close()

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %v", err)
	}

	pubPem := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}

	err = pem.Encode(pubFile, pubPem)
	if err != nil {
		return fmt.Errorf("failed to write public key to file: %v", err)
	}
	klog.Info("Public key saved to", publicKeyPath)

	return nil
}

func readKey(keyPath string) []byte {
	// get the abs
	// which will try to find the 'filename' from current workind dir too.
	pem, err := filepath.Abs(keyPath)
	if err != nil {
		klog.Exitf("failed to find absolute path for %s,err: %s", keyPath, err.Error())
	}

	// read the raw contents of the file
	data, err := os.ReadFile(pem)
	if err != nil {
		klog.Exitf(err.Error())
	}

	return data
}
