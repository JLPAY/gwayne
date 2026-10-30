package rsakey

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

var (
	RsaPrivateKey *rsa.PrivateKey
	RsaPublicKey  *rsa.PublicKey
	// RsaKid 是当前生效公钥的指纹，写入 JWT 的 kid 头，用于校验端强绑定密钥，
	// 防止旧密钥（含开源仓库默认密钥）签发的令牌在密钥轮换后继续可用。
	RsaKid string
)

// JwtIssuer 是服务端签发令牌时使用的 iss 声明，所有校验端必须一致。
const JwtIssuer = "gwayne"

// compromisedDefaultPubPEM_SHA256 是 Qihoo360/wayne 开源仓库自带的默认 RSA 公钥
// （conf/rsa-public.pem / apikey/rsa-public.pem 的初始内容）的 SHA256 指纹。
// 该私钥已随开源代码公开，任何人均可据此伪造管理员令牌，因此必须禁止使用。
const compromisedDefaultPubPEM_SHA256 = "2aa25af7280670152a45a9db8042a2d627201e8a5c7a45109cd223daf675c33c"

func InitRsaKey() {
	var err error

	privPath := config.Conf.App.RsaPrivateKey
	pubPath := config.Conf.App.RsaPublicKey

	// 防御：若当前公钥与开源默认公钥逐字节一致，说明仍在使用已被公开的密钥，
	// 存在越权风险，强制删除并重新生成密钥对。
	if isCompromisedDefaultKey(pubPath) {
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

	// 计算并缓存当前公钥指纹，供 JWT kid 绑定使用
	RsaKid = computeKid(RsaPublicKey)
	klog.Infof("当前 RSA 公钥指纹(kid)=%s", RsaKid)
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

// isCompromisedDefaultKey 判断给定公钥文件是否为开源默认公钥（按文件内容 SHA256 比对）。
func isCompromisedDefaultKey(pubPath string) bool {
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

// 生成 RSA 密钥对
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

	// 将证书保存为 PEM 格式文件
	//privateKeyPath := fmt.Sprintf("%s/rsa-private.pem", path)
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

	// 保存公钥到文件 (PEM格式)
	//publicKeyPath := fmt.Sprintf("%s/rsa-public.pem", path)
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

	//klog.Infof("rsa %s", pem)
	// read the raw contents of the file
	data, err := os.ReadFile(pem)
	if err != nil {
		klog.Exitf(err.Error())
	}

	return data
}
