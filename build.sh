#!/bin/bash
# 最小体积编译+自动UPX压缩
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o netcfg

if ! command -v upx &> /dev/null
then
    echo "未检测到upx，开始自动安装 upx-ucl"
    apt update && apt install upx-ucl -y
fi

echo "开始最高比例压缩二进制"
upx --best --lzma netcfg

chmod +x ./netcfg
echo "编译压缩完成，输出文件: ./netcfg"