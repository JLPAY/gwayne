package terminal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// @Title List terminal command rules
// @Description list all terminal command rules
// @router /terminal/command-rules [get]
func ListCommandRules(c *gin.Context) {
	rules, err := models.GetAllTerminalCommandRules()
	if err != nil {
		klog.Errorf("Failed to get command rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rules})
}

// @Title Get terminal command rule
// @Description get terminal command rule by id
// @router /terminal/command-rules/:id [get]
func GetCommandRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	rule, err := models.GetTerminalCommandRuleById(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// @Title Create terminal command rule
// @Description create a new terminal command rule
// @router /terminal/command-rules [post]
func CreateCommandRule(c *gin.Context) {
	// 读取原始请求体用于调试
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		klog.Errorf("Failed to read request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}
	
	// 恢复请求体供后续使用
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	
	klog.Infof("CreateCommandRule - Raw request body: %s", string(bodyBytes))

	// 使用 map 先接收 JSON，便于调试和灵活处理
	var jsonData map[string]interface{}
	if err := c.ShouldBindJSON(&jsonData); err != nil {
		klog.Errorf("Failed to bind JSON: %v, body: %s", err, string(bodyBytes))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format: " + err.Error(),
			"details": "Request body: " + string(bodyBytes),
		})
		return
	}

	klog.Infof("CreateCommandRule - Parsed JSON data: %+v", jsonData)

	// 构建规则对象
	rule := models.TerminalCommandRule{}

	// 解析 role
	if role, ok := jsonData["role"].(string); ok {
		rule.Role = role
		klog.Infof("Parsed role: %s", role)
	} else {
		klog.Errorf("Role field missing or invalid. jsonData: %+v", jsonData)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Role is required and must be a string",
			"received": jsonData,
		})
		return
	}

	// 解析 ruleType
	if ruleTypeVal, ok := jsonData["ruleType"]; ok {
		klog.Infof("ruleType value: %v (type: %T)", ruleTypeVal, ruleTypeVal)
		var ruleTypeInt int
		
		switch v := ruleTypeVal.(type) {
		case float64:
			ruleTypeInt = int(v)
			klog.Infof("Converted ruleType from float64: %d", ruleTypeInt)
		case int:
			ruleTypeInt = v
			klog.Infof("Converted ruleType from int: %d", ruleTypeInt)
		case int64:
			ruleTypeInt = int(v)
			klog.Infof("Converted ruleType from int64: %d", ruleTypeInt)
		case string:
			// 支持字符串类型的 ruleType
			parsed, err := strconv.Atoi(v)
			if err != nil {
				klog.Errorf("Failed to parse ruleType string '%s': %v", v, err)
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "ruleType must be a number (0 for blacklist, 1 for whitelist), got string: " + v,
				})
				return
			}
			ruleTypeInt = parsed
			klog.Infof("Converted ruleType from string '%s': %d", v, ruleTypeInt)
		default:
			klog.Errorf("Invalid ruleType type: %T, value: %v", v, v)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "ruleType must be a number (0 for blacklist, 1 for whitelist)",
				"received_type": fmt.Sprintf("%T", ruleTypeVal),
				"received_value": ruleTypeVal,
			})
			return
		}
		
		// 验证 ruleType 值范围
		if ruleTypeInt < 0 || ruleTypeInt > 1 {
			klog.Errorf("ruleType out of range: %d (must be 0 or 1)", ruleTypeInt)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "ruleType must be 0 (blacklist) or 1 (whitelist), got: " + strconv.Itoa(ruleTypeInt),
			})
			return
		}
		
		rule.RuleType = models.RuleType(ruleTypeInt)
		klog.Infof("Final ruleType: %d", rule.RuleType)
	} else {
		rule.RuleType = models.RuleTypeBlacklist // 默认黑名单
		klog.Infof("ruleType not provided, using default: %d (blacklist)", rule.RuleType)
	}

	// 解析 command
	if command, ok := jsonData["command"].(string); ok {
		rule.Command = command
		klog.Infof("Parsed command: %s", command)
	} else {
		klog.Errorf("Command field missing or invalid. jsonData: %+v", jsonData)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Command is required and must be a string",
			"received": jsonData,
		})
		return
	}

	// 解析 description (可选)
	if desc, ok := jsonData["description"].(string); ok {
		rule.Description = desc
		klog.Infof("Parsed description: %s", desc)
	} else {
		rule.Description = ""
		klog.Infof("Description not provided or not a string")
	}

	// 解析 enabled (可选，默认为 true)
	if enabled, ok := jsonData["enabled"].(bool); ok {
		rule.Enabled = enabled
		klog.Infof("Parsed enabled: %v", enabled)
	} else {
		rule.Enabled = true // 默认启用
		klog.Infof("Enabled not provided or not a bool, using default: true")
	}

	klog.Infof("Final parsed rule: Role=%s, RuleType=%d, Command=%s, Description=%s, Enabled=%v",
		rule.Role, rule.RuleType, rule.Command, rule.Description, rule.Enabled)

	id, err := models.AddTerminalCommandRule(&rule)
	if err != nil {
		klog.Errorf("Failed to create command rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rule.Id = id
	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// @Title Update terminal command rule
// @Description update terminal command rule
// @router /terminal/command-rules/:id [put]
func UpdateCommandRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	var rule models.TerminalCommandRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule.Id = id
	if err := models.UpdateTerminalCommandRule(&rule); err != nil {
		klog.Errorf("Failed to update command rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// @Title Delete terminal command rule
// @Description delete terminal command rule
// @router /terminal/command-rules/:id [delete]
func DeleteCommandRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	if err := models.DeleteTerminalCommandRule(id); err != nil {
		klog.Errorf("Failed to delete command rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted successfully"})
}

// @Title Get command rules by role
// @Description get command rules by role
// @router /terminal/command-rules/role/:role [get]
func GetCommandRulesByRole(c *gin.Context) {
	role := c.Param("role")
	if role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role is required"})
		return
	}

	rules, err := models.GetTerminalCommandRulesByRole(role)
	if err != nil {
		klog.Errorf("Failed to get command rules by role: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rules})
}

