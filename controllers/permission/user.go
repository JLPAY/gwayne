package permission

import (
	"github.com/JLPAY/gwayne/controllers/base"
	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"net/http"
	"strconv"
)

// @Title GetAll
// @Description get all user
// @Param	pageNo		query 	int	false		"the page current no"
// @Param	pageSize		query 	int	false		"the page size"
// @Success 200 {object} []models.User success
// @router / [get]
func UsersList(c *gin.Context) {
	// 构建 Kubernetes 查询参数
	param := base.BuildQueryParam(c)

	name := c.Query("name")
	if name != "" {
		param.Query["name__contains"] = name
	}

	// 获取总记录数
	total, err := models.GetTotal(new(models.User), param)
	if err != nil {
		klog.Errorf("Get Total users err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	users := []models.User{}
	err = models.GetAll(new(models.User), &users, param)
	if err != nil {
		klog.Errorf("Get all users err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": param.NewPage(total, users)})
}

// @Title Create
// @Description create user
// @Param	body		body 	models.User	true		"The user content"
// @Success 200 return models.User success
// @router / [post]
func UserCreate(c *gin.Context) {
	var user models.User
	if err := c.ShouldBind(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
	}

	_, err := models.AddUser(&user)
	if err != nil {
		klog.Errorf("Add user err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// @Title Get
// @Description find Object by id
// @Param	id		path 	int	true		"the id you want to get"
// @Success 200 {object} models.User success
// @router /:id [get]
func UserGet(c *gin.Context) {
	idStr := c.Param("id")

	// 将 id 字符串转换为 int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		// 如果转换失败，返回 400 错误
		klog.Errorf("Invalid id parameter: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid id parameter"})
		return
	}

	user, err := models.GetUserById(id)
	if err != nil {
		klog.Errorf("Get user err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// @Title Update
// @Description update the user
// @Param	id		path 	int	true		"The id you want to update"
// @Param	body		body 	models.User	true		"The body"
// @Success 200 models.User success
// @router /:id [put]
func UserUpdate(c *gin.Context) {
	var user models.User

	if err := c.ShouldBind(&user); err != nil {
		klog.Errorf("Invalid user parameter: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	err := models.UpdateUserById(&user)
	if err != nil {
		klog.Errorf("Update user err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// @Title Delete
// @Description delete the User
// @Param	id		path 	int	true		"The id you want to delete"
// @Success 200 {string} delete success!
// @router /:id [delete]
func UserDelete(c *gin.Context) {
	idStr := c.Param("id")

	// 将 id 字符串转换为 int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		// 如果转换失败，返回 400 错误
		klog.Errorf("Invalid id parameter: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid id parameter"})
		return
	}

	err = models.DeleteUser(id)
	if err != nil {
		klog.Errorf("Delete user err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": true})
}

// @Title Update
// @Description update the user admin
// @Param	id		path 	int	true		"The id you want to update"
// @Param	body		body 	Object	true		"The body"
// @Success 200 models.User success
// @router /:id/resetpassword [put]
func ResetPassword(c *gin.Context) {
	// 定义用户结构体，用于接收请求体中的数据
	var user *struct {
		Id       int64  `json:"id"`
		Password string `json:"password"`
	}

	// 解析请求体
	if err := c.ShouldBindJSON(&user); err != nil {
		klog.Errorf("user param error: %v", err)
		// 返回错误响应
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	err := models.ResetUserPassword(user.Id, user.Password)
	if err != nil {
		klog.Errorf("user reset password err: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// @Title Update
// @Description update the user admin
// @Param	id		path 	int	true		"The id you want to update"
// @Param	body		body 	models.User	true		"The body"
// @Success 200 models.User success
// @router /:id/admin [put]
func UpdateAdmin(c *gin.Context) {
	var user *models.User

	// 解析请求体
	if err := c.ShouldBindJSON(&user); err != nil {
		klog.Errorf("user param error: %v", err)
		// 返回错误响应
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	err := models.UpdateUserAdmin(user)
	if err != nil {
		klog.Errorf("user update admin err: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}
