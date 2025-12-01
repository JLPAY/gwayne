package myoauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
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

	// 打印原始用户信息响应
	klog.Infof("OAuth2 userinfo API response: %s", string(result))

	if len(o.ApiMapping) == 0 {
		err = json.Unmarshal(result, userinfo)
		if err != nil {
			return nil, fmt.Errorf("Error Unmarshal user info: %s", err)
		}
		klog.Infof("OAuth2 user info (no mapping): Name=%s, Email=%s, Display=%s", userinfo.Name, userinfo.Email, userinfo.Display)
	} else {
		// 如果有 API 映射，则使用映射从响应中提取用户信息
		usermap := make(map[string]interface{})
		if err := json.Unmarshal(result, &usermap); err != nil {
			return nil, fmt.Errorf("Error Unmarshal user info: %s", err)
		}
		klog.Infof("OAuth2 userinfo raw data: %+v", usermap)
		klog.Infof("OAuth2 API mapping: %+v", o.ApiMapping)

		if usermap[o.ApiMapping["name"]] != nil {
			userinfo.Name = usermap[o.ApiMapping["name"]].(string)
		}
		if usermap[o.ApiMapping["email"]] != nil {
			userinfo.Email = usermap[o.ApiMapping["email"]].(string)
		}
		if usermap[o.ApiMapping["display"]] != nil {
			userinfo.Display = usermap[o.ApiMapping["display"]].(string)
		}
		klog.Infof("OAuth2 user info (with mapping): Name=%s, Email=%s, Display=%s", userinfo.Name, userinfo.Email, userinfo.Display)
	}

	return userinfo, nil
}
