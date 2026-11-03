package node

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"
)

// GetCertExpiry 对 hostport（如 10.0.0.1:10250 或 api:6443）发起 TLS 握手，
// 读取对端证书（InsecureSkipVerify 仅用于读取证书，不做信任校验），返回证书过期时间与主题。
// 用于巡检 kubelet / API Server serving 证书的剩余有效期。
func GetCertExpiry(hostport string) (notAfter time.Time, subject string, err error) {
	if hostport == "" {
		return time.Time{}, "", fmt.Errorf("证书巡检：host 为空")
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true, // 仅读取证书，不校验信任链
		MinVersion:         tls.VersionTLS12,
	}
	conn, err := tls.DialWithDialer(d, "tcp", hostport, tlsCfg)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("TLS 握手失败 %s: %v", hostport, err)
	}
	defer conn.Close()
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return time.Time{}, "", fmt.Errorf("证书巡检 %s：对端未返回证书", hostport)
	}
	cert := state.PeerCertificates[0]
	subj := cert.Subject.CommonName
	if subj == "" && len(cert.DNSNames) > 0 {
		subj = cert.DNSNames[0]
	}
	return cert.NotAfter, subj, nil
}

// APIServerHostPort 从 rest.Config.Host（如 https://10.0.0.1:6443）解析出 host:port。
func APIServerHostPort(host string) string {
	if host == "" {
		return ""
	}
	u, err := url.Parse(host)
	if err != nil {
		return host
	}
	h := u.Host
	if h == "" {
		h = host
	}
	if u.Port() == "" {
		if u.Scheme == "https" {
			h += ":443"
		} else if u.Scheme == "http" {
			h += ":80"
		}
	}
	return h
}
