package config

import (
	"crypto/tls"
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// 全局配置变量
var Conf = new(Config)

type Config struct {
	App      AppConf  `ini:"App"`
	DataBase DataBase `ini:"DataBase"`
	Log      LogConf  `ini:"Log"`
	Auth     Auth     `ini:"Auth"`
}

type AppConf struct {
	Name          string `ini:"Name"`
	HttpPort      int    `ini:"HttpPort"`
	AppUrl        string `ini:"AppUrl"`
	BetaUrl       string `ini:"BetaUrl"`
	RunMode       string `ini:"RunMode"`
	RsaPrivateKey string `ini:"RsaPrivateKey"`
	RsaPublicKey  string `ini:"RsaPublicKey"`
	TokenLifeTime int64  `ini:"TokenLifeTime"`
	AppKey        string `ini:"AppKey"`
}

type DataBase struct {
	Driver     string `ini:"Driver"`
	DBName     string `ini:"DBName"`
	Host       string `ini:"Host"`
	Port       int    `ini:"Port"`
	DBUser     string `ini:"DBUser"`
	DBPassword string `ini:"DBPassword"`
	DBConnTTL  int    `ini:"DBConnTTL"`
	ShowSql    bool   `ini:"ShowSql"`
	LogMode    bool   `ini:"LogMode"`
}

type LogConf struct {
	LogLevel string `ini:"LogLevel"`
	LogPath  string `ini:"LogPath"`
}

type Auth struct {
	Oauth2 Oauth2Conf `ini:"Oauth2"`
	Ldap   LdapConf   `ini:"Ldap"`
}

type Oauth2Conf struct {
	Enabled      bool   `ini:"Enabled"`
	Name         string `ini:"Name"` // OAuth2 服务名称，用于区分多个认证服务
	ClientId     string `ini:"ClientId"`
	ClientSecret string `ini:"ClientSecret"`
	RedirectURL  string `ini:"RedirectURL"`
	AuthURL      string `ini:"AuthURL"`
	TokenURL     string `ini:"TokenURL"`
	ApiURL       string `ini:"ApiURL"`
	Scopes       string `ini:"Scopes"`
	ApiMapping   string `ini:"ApiMapping"`
}

type LdapConf struct {
	Enabled   bool        `ini:"Enabled"`
	Url       string      `ini:"Url"`
	BaseDN    string      `ini:"BaseDN"`
	BindDN    string      `ini:"BindDN"`
	Password  string      `ini:"Password"`
	UseSSL    bool        `ini:"UseSSL"`
	SkipTLS   bool        `ini:"SkipTLS"`
	TLSConfig *tls.Config `ini:"TLSConfig"`
	CertFile  string      `ini:"CertFile"`
	KeyFile   string      `ini:"KeyFile"`
	CAFile    string      `ini:"CAFile"`
	Filter    string      `ini:"Filter"`
	Uid       string      `ini:"Uid"`
	Scope     string      `ini:"Scope"`
}

// 设置读取配置信息
func init() {
	viper.SetConfigName("app")
	viper.SetConfigType("ini")  // 设置为 ini 格式
	viper.AddConfigPath("conf") // 配置文件路径

	// 读取配置信息
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("读取配置文件失败:%s", err))
	}

	// 热更新配置
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		// 将读取的配置信息保存至全局变量 Conf
		if err := viper.Unmarshal(Conf); err != nil {
			panic(fmt.Errorf("初始化配置文件失败:%s", err))
		}
	})

	// 将读取的配置信息保存至全局变量 Conf
	if err := viper.Unmarshal(Conf); err != nil {
		panic(fmt.Errorf("初始化配置文件失败:%s", err))
	}
}

// 获取全局配置
func GetConfig() Config {
	return *Conf
}
