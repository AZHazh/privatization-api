# 当前分支 Docker 部署说明

这套流程用于把当前分支构建成镜像，并让其他服务器在没有源码的情况下部署。

## GitHub Actions 自动构建并推送到阿里云 ACR

### 1. 配置 GitHub Secrets

进入 GitHub 仓库：

```text
Settings -> Secrets and variables -> Actions -> New repository secret
```

新增 3 个 Secret：

```text
ALIYUN_REGISTRY
```

值：

```text
crpi-d2emme9fdsxy80iz.cn-hangzhou.personal.cr.aliyuncs.com
```

```text
ALIYUN_USERNAME
```

值填写阿里云 ACR 访问凭证里的登录用户名，例如：

```text
gt鬼瞳丶
```

```text
ALIYUN_PASSWORD
```

值填写阿里云 ACR 访问凭证里的固定密码。

### 2. 推送当前分支到 GitHub

确保当前分支已经推送到 GitHub。GitHub Actions 只能构建 GitHub 上已有的代码。

### 3. 手动运行 workflow

进入 GitHub 仓库：

```text
Actions -> Build and Push Aliyun ACR -> Run workflow
```

在 `Use workflow from` 里选择你要构建的分支。

参数建议：

```text
image_tag: 20260601
image_repository: tiandou/tian-dou
platforms: linux/amd64
upload_deploy_bundle: true
```

运行成功后，镜像地址是：

```text
crpi-d2emme9fdsxy80iz.cn-hangzhou.personal.cr.aliyuncs.com/tiandou/tian-dou:20260601
```

同时在本次 Actions 运行页面的 `Artifacts` 里下载：

```text
sub2api-deploy-bundle-20260601
```

里面就是目标服务器部署包 `sub2api-deploy-bundle.tar.gz`。

### 4. 第三方服务器部署

第三方服务器不需要源码，只需要部署包和镜像地址：

```bash
tar xzf sub2api-deploy-bundle.tar.gz
cd sub2api-deploy

docker login --username='gt鬼瞳丶' crpi-d2emme9fdsxy80iz.cn-hangzhou.personal.cr.aliyuncs.com

SUB2API_IMAGE=crpi-d2emme9fdsxy80iz.cn-hangzhou.personal.cr.aliyuncs.com/tiandou/tian-dou:20260601 \
SUB2API_INSTANCE=tiandou-prod \
SUB2API_PORT=8080 \
./docker-deploy.sh

docker compose up -d
```

同一台服务器部署多套时，换 `SUB2API_INSTANCE` 和 `SUB2API_PORT`。

## 1. 在有源码的机器上构建并推送镜像

```bash
./deploy/build-and-push-image.sh registry.example.com/sub2api:20260601
```

如果只想本机测试构建，不推送：

```bash
PUSH=false ./deploy/build-and-push-image.sh local/sub2api:test
```

## 2. 打包部署文件

目标服务器只需要 Docker 和 Docker Compose v2，不需要源码。

在有源码的机器上生成部署包：

```bash
./deploy/package-deploy-bundle.sh
```

把生成的 `sub2api-deploy-bundle.tar.gz` 传到目标服务器。

## 3. 在目标服务器部署

```bash
tar xzf sub2api-deploy-bundle.tar.gz
cd sub2api-deploy
SUB2API_IMAGE=registry.example.com/sub2api:20260601 \
SUB2API_INSTANCE=sub2api-prod \
SUB2API_PORT=8080 \
./docker-deploy.sh
docker compose up -d
```

如果部署文件已经合并到远端分支，也可以直接在线拉脚本：

```bash
mkdir -p sub2api-prod
cd sub2api-prod
SUB2API_IMAGE=registry.example.com/sub2api:20260601 \
SUB2API_INSTANCE=sub2api-prod \
SUB2API_PORT=8080 \
bash -c "$(curl -fsSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh)"
docker compose up -d
```

## 4. 同一台服务器部署多套

每一套使用独立目录、独立 `SUB2API_INSTANCE`、独立 `SUB2API_PORT`：

```bash
mkdir -p sub2api-test-a
cd sub2api-test-a
SUB2API_IMAGE=registry.example.com/sub2api:20260601 \
SUB2API_INSTANCE=sub2api-test-a \
SUB2API_PORT=18080 \
bash -c "$(curl -fsSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh)"
docker compose up -d

cd ..
mkdir -p sub2api-test-b
cd sub2api-test-b
SUB2API_IMAGE=registry.example.com/sub2api:20260601 \
SUB2API_INSTANCE=sub2api-test-b \
SUB2API_PORT=18081 \
bash -c "$(curl -fsSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh)"
docker compose up -d
```

`COMPOSE_PROJECT_NAME` 会隔离容器、网络和命名卷；本地目录版还会把数据保存在当前部署目录。

## 5. 日志和数据

```bash
docker compose logs -f sub2api
tail -f data/logs/sub2api.log
```

本地目录：

- `data/`：应用数据、配置和文件日志
- `postgres_data/`：PostgreSQL 数据
- `redis_data/`：Redis 数据

## 6. 升级

```bash
docker compose pull
docker compose up -d
```
