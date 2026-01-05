package myldap

import (
	"errors"
	"fmt"
	ldap "github.com/go-ldap/ldap/v3"
	"strings"
)

// FetchUsers 从 LDAP 服务器获取用户
func (c *LDAPClient) FetchUsers() (map[string]*ldap.Entry, error) {
	searchRequest := ldap.NewSearchRequest(
		c.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(&(objectClass=Person))",
		[]string{},
		nil,
	)

	sr, err := c.Conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("查询%s的%s所有用户出错: %v\n", c.URL, c.BaseDN, err)
	}

	users := make(map[string]*ldap.Entry)
	for _, entry := range sr.Entries {
		users[entry.DN] = entry
	}

	return users, nil
}

func (c *LDAPClient) SearchUsers(filter, username string) ([]*ldap.Entry, error) {

	attributes := []string{"uid", "cn", "mail", "email"}

	filterString := strings.Replace(filter, "%s", username, -1)
	//klog.Infof("filterString: %s", filterString)

	// 查询用户信息（根据实际情况调整查询方式）
	searchRequest := ldap.NewSearchRequest(
		c.BaseDN,               // 替换为您的实际 LDAP 路径
		ldap.ScopeWholeSubtree, // 搜索范围
		ldap.NeverDerefAliases, // 别名处理
		0,                      // 结果大小限制
		0,                      // 时间限制
		false,                  // 仅返回属性
		filterString,           // 查询条件
		attributes,             // 返回的属性
		nil,
	)

	result, err := c.Conn.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(result.Entries) == 0 {
		return nil, errors.New("user not found")
	}

	return result.Entries, nil
}

// dn查用户在openldap是否存在
func (c *LDAPClient) EnsureDNExists(dn string) (int, error) {
	// 根据dn查用户在openldap是否存在
	searchTargetRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
		"(&(objectClass=Person))",
		[]string{"dn"},
		nil,
	)

	targetSearchResult, err := c.Conn.Search(searchTargetRequest)
	if err != nil {
		// DN不存在时(No Such Object 错误)
		if ldapError, ok := err.(*ldap.Error); ok && ldapError.ResultCode == ldap.LDAPResultNoSuchObject {
			//fmt.Printf("DN不存在时,%s %d", err.Error(), ldapError.ResultCode)
			return 0, nil
		}

		return 0, fmt.Errorf("查询%s的%s用户失败 %v\n", c.URL, dn, err)
	}

	return len(targetSearchResult.Entries), nil

}

// uid查用户在openldap是否存在
func (c *LDAPClient) EnsureUidExists(uid string) (int, error) {
	// 根据dn查用户在openldap是否存在
	searchTargetRequest := ldap.NewSearchRequest(
		c.BaseDN,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
		//"(&(objectClass=Person))",
		fmt.Sprintf("(&(objectClass=Person)(uid=%s))", uid),
		[]string{"dn"},
		nil,
	)

	targetSearchResult, err := c.Conn.Search(searchTargetRequest)
	if err != nil {
		// DN不存在时(No Such Object 错误)
		if ldapError, ok := err.(*ldap.Error); ok && ldapError.ResultCode == ldap.LDAPResultNoSuchObject {
			//fmt.Printf("DN不存在时,%s %d", err.Error(), ldapError.ResultCode)
			return 0, nil
		}

		return 0, fmt.Errorf("查询%s的%s用户失败 %v\n", c.URL, uid, err)
	}

	return len(targetSearchResult.Entries), nil

}

func (c *LDAPClient) AddUser(entry *ldap.Entry) error {
	// 根据Entry 创建用户
	addRequest := ldap.NewAddRequest(entry.DN, nil)
	// 同步openldap
	addRequest.Attribute("objectClass", []string{"top", "person", "organizationalPerson", "inetOrgPerson"})
	addRequest.Attribute("cn", []string{entry.GetAttributeValue("cn")})
	addRequest.Attribute("sn", []string{entry.GetAttributeValue("sn")})
	addRequest.Attribute("uid", []string{entry.GetAttributeValue("uid")})
	addRequest.Attribute("displayName", []string{entry.GetAttributeValue("displayName")})
	addRequest.Attribute("mail", []string{entry.GetAttributeValue("mail")})
	addRequest.Attribute("employeeNumber", []string{entry.GetAttributeValue("employeeNumber")})
	if len(entry.GetAttributeValues("telephoneNumber")) > 0 {
		addRequest.Attribute("telephoneNumber", []string{entry.GetAttributeValue("telephoneNumber")})
	}
	addRequest.Attribute("userPassword", []string{entry.GetAttributeValue("userPassword")})

	err := c.Conn.Add(addRequest)
	if err != nil {
		//klog.Errorf("添加用户失败: %s\n", entry.DN)
		return fmt.Errorf("添加用户%s失败: %v\n", entry.DN, err)
	}
	//klog.Infof("添加用户成功: %s\n", entry.DN)
	return nil
}

func (c *LDAPClient) UpdateUser(entry *ldap.Entry) error {
	// 根据Entry 更新用户
	modifyRequest := ldap.NewModifyRequest(entry.DN, nil)
	//modifyRequest.Replace("cn", []string{entry.GetAttributeValue("cn")})
	//modifyRequest.Replace("sn", []string{entry.GetAttributeValue("sn")})
	//modifyRequest.Replace("uid", []string{entry.GetAttributeValue("uid")})
	//modifyRequest.Replace("displayName", []string{entry.GetAttributeValue("displayName")})
	//modifyRequest.Replace("mail", []string{entry.GetAttributeValue("mail")})
	//modifyRequest.Replace("employeeNumber", []string{entry.GetAttributeValue("employeeNumber")})
	//modifyRequest.Replace("telephoneNumber", []string{entry.GetAttributeValue("telephoneNumber")})
	modifyRequest.Replace("userPassword", []string{entry.GetAttributeValue("userPassword")})
	err := c.Conn.Modify(modifyRequest)
	if err != nil {
		//klog.Errorf("更新用户失败: %s\n", entry.DN)
		return fmt.Errorf("更新用户%s失败: %v\n", entry.DN, err)
	}
	//klog.Infof("添加用户成功: %s\n", entry.DN)
	return nil
}

// 删除用户
func (c *LDAPClient) DeleteEntry(dn string) error {
	deleteRequest := ldap.NewDelRequest(dn, nil)
	return c.Conn.Del(deleteRequest)
}
