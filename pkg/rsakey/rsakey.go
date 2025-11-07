package rsakey

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/dgrijalva/jwt-go"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

var (
	RsaPrivateKey *rsa.PrivateKey
	RsaPublicKey  *rsa.PublicKey
)

func InitRsaKey() {
	var err error

	// 检查密钥对是否存在，不存在生成密钥对
	if !keyExists(config.Conf.App.RsaPublicKey) || !keyExists(config.Conf.App.RsaPrivateKey) {
		klog.Info("rsa密钥对不存在，生成新的密钥对...")
		err := generateRsaKeyPair(2048, "./conf")
		if err != nil {
			klog.Errorf("生成密钥对失败 %s", err)
		}
	}

	RsaPrivateKey, err = jwt.ParseRSAPrivateKeyFromPEM(readKey(config.Conf.App.RsaPrivateKey))
	if err != nil {
		klog.Exitf("载入密钥: %s 失败 %s", config.Conf.App.RsaPrivateKey, err)
	}
	klog.Infof("载入密钥: %s 完成", config.Conf.App.RsaPrivateKey)

	RsaPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(readKey(config.Conf.App.RsaPublicKey))
	if err != nil {
		klog.Exitf("载入公钥: %s 失败 %s", config.Conf.App.RsaPublicKey, err)
	}
	klog.Infof("载入公钥: %s 完成", config.Conf.App.RsaPublicKey)
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
