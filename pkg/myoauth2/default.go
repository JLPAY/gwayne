package myoauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/oauth2"
	"io/ioutil"
)

// _ 是空白标识符，确保 OAuth2Default 类型实现了 OAuther 接口
var _ OAuther = &OAuth2Default{}

type OAuth2Default struct {
	*oauth2.Config
	ApiUrl     string
	ApiMapping map[string]string
}

// 用于获取用户信息，接受 token，获取用户信息
func (o *OAuth2Default) UserInfo(token *oauth2.Token) (*BasicUserInfo, error) {
	userinfo := &BasicUserInfo{}

	client := o.Client(context.Background(), token)
	userInfoResp, err := client.Get(o.ApiUrl)
	if err != nil {
		return userinfo, err
	}
	defer userInfoResp.Body.Close()

	result, err := ioutil.ReadAll(userInfoResp.Body)
	if err != nil {
		return nil, err
	}

	//klog.Infof("%v", string(result))

	if len(o.ApiMapping) == 0 {
		err = json.Unmarshal(result, userinfo)
		if err != nil {
			return nil, fmt.Errorf("Error Unmarshal user info: %s", err)
		}
	} else {
		// 如果有 API 映射，则使用映射从响应中提取用户信息
		usermap := make(map[string]interface{})
		if err := json.Unmarshal(result, &usermap); err != nil {
			return nil, fmt.Errorf("Error Unmarshal user info: %s", err)
		}
		if usermap[o.ApiMapping["name"]] != nil {
			userinfo.Name = usermap[o.ApiMapping["name"]].(string)
		}
		if usermap[o.ApiMapping["email"]] != nil {
			userinfo.Email = usermap[o.ApiMapping["email"]].(string)
		}
		if usermap[o.ApiMapping["display"]] != nil {
			userinfo.Display = usermap[o.ApiMapping["display"]].(string)
		}
	}

	return userinfo, nil
}
