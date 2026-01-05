package myldap

import (
    "fmt"
    "strings"

    ldap "github.com/go-ldap/ldap/v3"
    klog "k8s.io/klog/v2"
)

// 递归创建 OU
func (c *LDAPClient) EnsureOUExists(dn string) error {
    parts := strings.Split(dn, ",")
    // klog.Infof("OU: %v", parts)

    var currentDN string

    for i := len(parts) - 1; i >= 0; i-- {
        part := strings.TrimSpace(parts[i])

        if currentDN != "" {
            currentDN = part + "," + currentDN
        } else {
            currentDN = part
        }

        //fmt.Printf("OU %d currentDN: %s\n", i, currentDN)
        if strings.HasPrefix(currentDN, "dc=") || strings.HasPrefix(currentDN, "DC=")|| strings.HasPrefix(currentDN, "cn=") || strings.HasPrefix(currentDN, "CN=") {
            //klog.Infof("不用创建OU: %s,DN: %s", strings.Split(part, "=")[1] ,currentDN)
            continue
        }

        searchRequest := ldap.NewSearchRequest(
            currentDN,
            ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
            "(objectClass=organizationalUnit)",
            []string{"dn"},
            nil,
        )

        result, err := c.Conn.Search(searchRequest)
        if err != nil {
            if ldapError, ok := err.(*ldap.Error); ok && ldapError.ResultCode == ldap.LDAPResultNoSuchObject {
                klog.Infof("创建OU: %s,DN: %s", strings.Split(part, "=")[1] ,currentDN)
                addRequest := ldap.NewAddRequest(currentDN, nil)
                addRequest.Attribute("objectClass", []string{"organizationalUnit"})
                addRequest.Attribute("ou", []string{strings.Split(part, "=")[1]})

                err = c.Conn.Add(addRequest)
                if err != nil {
                    return fmt.Errorf("failed to addOU %s: %v", currentDN, err)
                }
                continue
            }

            return fmt.Errorf("查询ou: %s出错: %v", currentDN, err)
        }

        if len(result.Entries) == 0 {
            klog.Infof("创建OU: %s,DN: %s", strings.Split(part, "=")[1] ,currentDN)
            addRequest := ldap.NewAddRequest(currentDN, nil)
            addRequest.Attribute("objectClass", []string{"organizationalUnit"})
            addRequest.Attribute("ou", []string{strings.Split(part, "=")[1]})

            err = c.Conn.Add(addRequest)
            if err != nil {
                return fmt.Errorf("failed to addOU %s: %v", currentDN, err)
            }
        }
    }
    return nil
}
