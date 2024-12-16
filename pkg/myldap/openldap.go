package myldap

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"io/ioutil"

	ldap "github.com/go-ldap/ldap/v3"
)

type LDAPClient struct {
	URL          string      `yaml:"url"`
	BindDN       string      `yaml:"bindDN"`
	BaseDN       string      `yaml:"BaseDN"`
	BindPassword string      `yaml:"BindPassword"`
	UseSSL       bool        `yaml:"UseSSL"`
	SkipTLS      bool        `yaml:"SkipTLS"`
	TLSConfig    *tls.Config `yaml:"TLSConfig"`
	CertFile     string      `yaml:"CertFile"`
	KeyFile      string      `yaml:"KeyFile"`
	CAFile       string      `yaml:"CAFile"`
	Conn         *ldap.Conn  `yaml:"Conn"`
}

// 创建一个新的 LDAP 客户端实例
func NewLDAPClient(ldapconf config.LdapConf) (*LDAPClient, error) {
	var tlsConfig *tls.Config

	if ldapconf.UseSSL || ldapconf.SkipTLS {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: ldapconf.SkipTLS,
		}

		if ldapconf.CAFile != "" {
			caCert, err := ioutil.ReadFile(ldapconf.CAFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA file: %v", err)
			}

			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
		}

		if ldapconf.CertFile != "" && ldapconf.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(ldapconf.CertFile, ldapconf.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load certificate and key: %v", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	// Create LDAP connection
	conn, err := ldap.DialURL(ldapconf.Url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %v", err)
	}

	// Apply TLS if needed
	if tlsConfig != nil {
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	// Apply TLS if needed
	if tlsConfig != nil {
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	// Bind to LDAP server
	if err := conn.Bind(ldapconf.BindDN, ldapconf.Password); err != nil {
		conn.Close()
		return nil, fmt.Errorf("初始化ldapClient失败, failed to bind to LDAP server: %v", err)
	}

	return &LDAPClient{
		URL:          ldapconf.Url,
		BindDN:       ldapconf.BindDN,
		BaseDN:       ldapconf.BaseDN,
		BindPassword: ldapconf.Password,
		Conn:         conn,
		TLSConfig:    tlsConfig,
		CertFile:     ldapconf.CertFile,
		KeyFile:      ldapconf.KeyFile,
		CAFile:       ldapconf.CAFile,
		UseSSL:       ldapconf.UseSSL,
		SkipTLS:      ldapconf.SkipTLS,
	}, nil

}

// Close 关闭 LDAP 连接
func (c *LDAPClient) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
}

func loadCAFile(caFile string) (*x509.CertPool, error) {
	// 读取 CA 文件
	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err // 返回文件读取错误
	}

	// 创建新的证书池
	caCertPool := x509.NewCertPool()

	// 将 CA 证书添加到证书池中
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return caCertPool, nil
}
