package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/crd"
	"github.com/JLPAY/gwayne/controllers/kubernetes/proxy"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupKubernetesProxyResourcesRoutes(rg *gin.RouterGroup) {
	// /apps/:appid([0-9]+)/_proxy/clusters/ 路由
	// For Kubernetes resource router
	// appid used to check permission
	proxyResourceGroup := rg.Group("/apps/:appid([0-9]+)/_proxy/clusters/:cluster").Use(middleware.JWTauth())
	{
		// 不带 namespace 的资源
		// 获取 kind 资源列表
		proxyResourceGroup.GET("/:kind", proxy.List)
		// 创建kind
		proxyResourceGroup.POST("/:kind", proxy.Create)
		//  获取指定 kind
		proxyResourceGroup.GET("/:kind/:name", proxy.Get)
		// update
		proxyResourceGroup.PUT("/:kind/:name", proxy.Put)
		proxyResourceGroup.DELETE("/:kind/:name", proxy.Delete)

		// namespaces 为kind时，获取所有 namespaces 列表和详情
		proxyResourceGroup.GET("/namespaces", proxy.NamespacesList)
		proxyResourceGroup.GET("/namespaces/names", proxy.GetNames) // 获取所有 namespaces名称

		// crds 为kind时
		proxyResourceGroup.GET("/customresourcedefinitions", crd.List)
		proxyResourceGroup.POST("/customresourcedefinitions", crd.Create)
		proxyResourceGroup.GET("/customresourcedefinitions/:name", crd.Get)
		proxyResourceGroup.PUT("/customresourcedefinitions/:name", crd.Update)
		proxyResourceGroup.DELETE("/customresourcedefinitions/:name", crd.Delete)

		// 处理每类crd资源
		proxyResourceGroup.GET("/apis/:group/:version/:kind", crd.CRDList)
		proxyResourceGroup.POST("/apis/:group/:version/:kind", crd.CRDCreate)
		proxyResourceGroup.GET("/apis/:group/:version/:kind/:name", crd.CRDGet)
		proxyResourceGroup.PUT("/apis/:group/:version/:kind/:name", crd.CRDUpdate)
		proxyResourceGroup.DELETE("/apis/:group/:version/:kind/:name", crd.CRDDelete)
		proxyResourceGroup.GET("/apis/:group/:version/namespaces/:namespacesName/:kind/:name", crd.CRDGet)
		proxyResourceGroup.PUT("/apis/:group/:version/namespaces/:namespacesName/:kind/:name", crd.CRDUpdate)
		proxyResourceGroup.DELETE("/apis/:group/:version/namespaces/:namespacesName/:kind/:name", crd.CRDDelete)

		proxyResourceGroup.POST("/namespaces", proxy.NamespacesCreate)                  // 创建namespace
		proxyResourceGroup.GET("/namespaces/:namespaceName", proxy.NamespacesGet)       // 获取指定namespace
		proxyResourceGroup.POST("/namespaces/:namespaceName", proxy.NamespacesPut)      // update 指定namespace
		proxyResourceGroup.DELETE("/namespaces/:namespaceName", proxy.NamespacesDelete) // Delete 指定namespace

		// 带 namespace 的资源
		proxyResourceGroup.GET("/namespaces/:namespaceName/:kind", proxy.List)
		proxyResourceGroup.POST("/namespaces/:namespaceName/:kind", proxy.Create)
		proxyResourceGroup.GET("/namespaces/:namespaceName/:kind/:name", proxy.Get)
		proxyResourceGroup.PUT("/namespaces/:namespaceName/:kind/:name", proxy.Put)
		proxyResourceGroup.DELETE("/namespaces/:namespaceName/:kind/:name", proxy.Delete)

	}
}
