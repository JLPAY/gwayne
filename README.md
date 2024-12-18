# gwayne

#### 介绍
k8s 多集群管理平台 
基于 `https://github.com/360yun/wayne.git`
，是一个通用的、基于 Web 的 Kubernetes 多集群管理平台。通过可视化 Kubernetes 对象模板编辑的方式，降低业务接入成本， 拥有完整的权限管理系统，适应多租户场景，是一款适合企业级集群使用的发布平台。

![overview.png](doc/images/overview.png)

![nodes.jpg](doc/images/nodes.jpg)

#### 软件架构
软件架构说明


#### 安装教程

1.  docker-compose 部署
```shell
 git clone https://github.com/JLPAY/gwayne.git
 cd gwayne/hack/docker-compose
 # 修改配置文件 conf/config.js 的 URL,ip为分部署节点的ip 
 docker-compose up -d
 
 # 访问 http://ip:8080  默认密码 admin/admin
```
2.  k8s 部署，manifests文件 hack/kubernetes



#### 使用说明

* 项目依赖

    * Golang 1.12+ (installation manual)
    * Docker 17.05+ (installation manual)
    * gin (installation manual)
    * MySQL 5.6+ (Wayne 主要数据都存在 MySQL 中)


#### 参与贡献

1.  Fork 本仓库
2.  新建 Feat_xxx 分支
3.  提交代码
4.  新建 Pull Request

