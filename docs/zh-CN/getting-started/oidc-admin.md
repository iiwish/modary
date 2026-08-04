# OIDC Admin 教程

OIDC 是 Admin Profile 的显式创建选项。它会替换本地密码登录，而不是在后台保留
一条隐藏的密码路由；业务 Module、RBAC、产品 Scope 和 React 工作台不需要因此
改变。

## 创建项目

```bash
go run github.com/iiwish/modary/cmd/modary@v0.3.0-alpha.1 \
  new operations-admin --profile admin --with oidc \
  --module example.com/acme/operations-admin
cd operations-admin
go mod tidy
```

可以重复使用 `--with`，组合 `oidc`、`tasks`、`audit` 和 `otel`。生成结果会包含
`components/oidc` 与 OIDC HTTP contribution，不包含密码凭据配置、本地密码登录
路由、密码表单或 Password capability。

## 配置身份提供方

在身份提供方只登记一个精确回调地址：

```text
https://admin.example.com/api/auth/oidc/callback
```

提供方需要支持 OpenID Connect discovery 和 Authorization Code flow。Modary
固定使用 state、nonce、PKCE S256，并校验 issuer、签名、audience 和时间声明。
除明确启用本地不安全选项外，issuer 与 redirect URL 都必须使用 HTTPS。

```bash
export DATABASE_URL='postgres://app:...@db:5432/app?sslmode=require'
export MODARY_OIDC_ISSUER_URL='https://id.example.com'
export MODARY_OIDC_CLIENT_ID='operations-admin'
export MODARY_OIDC_CLIENT_SECRET='...'
export MODARY_OIDC_REDIRECT_URL='https://admin.example.com/api/auth/oidc/callback'
export MODARY_OIDC_SUBJECT='provider-stable-subject'
```

## 身份、Scope 与权限

生成样例把一个精确的 provider subject 映射到已经创建的 `admin` principal。
subject 只在对应 issuer 内稳定。email、display name、group、role、tenant 和 scope
claim 都不会直接授予 Modary 权限。

真实产品应维护经过评审的 issuer/subject 到 principal 的精确映射。RBAC 再把
principal 分别绑定到产品 Scope。一个 principal 可以没有 Scope、绑定一个 Scope
或绑定多个 Scope；未映射 subject 和未绑定 Scope 都必须拒绝。

## 会话与验收

回调成功后创建可撤销的服务端应用会话。Cookie 默认启用 host-only、HttpOnly、
SameSite 和 Secure。应用退出只撤销 Modary 会话；上游单点退出属于提供方特定能力。

本 Alpha 中，尚未完成的 OIDC 跳转流程有容量上限，并保存在发起登录的进程内。
Callback 需要通过单登录实例或入口会话亲和性回到该实例。已建立的应用 Session
保存在 PostgreSQL 中，可由任意应用实例解析；v0.3 不包含共享流程存储。

公开部署前，应验证登录、当前会话、退出、会话撤销、重启、多 Scope 授权和未绑定
Scope 拒绝，并覆盖 state、nonce、issuer、audience、过期、重放、重复参数、超大
响应、discovery/JWKS 故障和未映射 subject。MFA、注册、恢复、目录同步与上游风控
仍由身份提供方和产品负责。
