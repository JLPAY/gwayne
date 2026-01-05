package models

const (
	AuthTypeDB     = "db"
	AuthTypeOAuth2 = "oauth2"
	AuthTypeLDAP   = "ldap"
)

// AuthModel holds information used to authenticate.
type AuthModel struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	OAuth2Name string `json:"oauth2_name"` // name属性，用于区分不同的回调接口
	OAuth2Code string `json:"oauth2_code"`
}
