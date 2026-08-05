#!/bin/bash
# 最小体积编译+自动UPX压缩

# 获取版本号：优先取最近的 git tag，否则使用 commit 短哈希
if git describe --tags --always --dirty > /dev/null 2>&1; then
    VERSION=$(git describe --tags --always --dirty)
else
    VERSION="dev"
fi

echo "当前版本号: $VERSION"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$VERSION" -o netcfg

if ! command -v upx &> /dev/null
then
    echo "未检测到upx，开始自动安装 upx-ucl"
    apt update && apt install upx-ucl -y
fi

echo "开始最高比例压缩二进制"
upx --best --lzma netcfg

chmod +x ./netcfg
echo "编译压缩完成，输出文件: ./netcfg"