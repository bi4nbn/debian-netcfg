#!/bin/bash
set -e

echo "========== 查看本地变更 =========="
git status

echo -e "\n========== 全部加入暂存区 =========="
git add .

read -p "请输入本次提交备注: " commitMsg
if [ -z "$commitMsg" ]; then
    commitMsg="日常更新"
fi

echo -e "\n========== 提交变更 =========="
git commit -m "$commitMsg"

# ---------- 自动递增 patch 版本号 ----------
latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo "当前最新标签: $latest_tag"

# 解析当前版本号
base=${latest_tag#v}
IFS='.' read -r -a parts <<< "$base"
major=${parts[0]:-0}
minor=${parts[1]:-0}
patch=${parts[2]:-0}

# patch 自动 +1
new_patch=$((patch + 1))
new_version="v$major.$minor.$new_patch"

echo "自动生成新标签: $new_version"
git tag "$new_version"
echo "已创建标签 $new_version"

# ---------- 拉取与推送 ----------
echo -e "\n========== 拉取远程最新避免冲突 =========="
git pull origin main --rebase

echo -e "\n========== 推送到GitHub main分支 =========="
git push origin main

echo "推送标签 $new_version ..."
git push origin "$new_version"

# ---------- 编译二进制 ----------
echo -e "\n========== 编译二进制（带版本号 $new_version） =========="
./build.sh

echo -e "\n✅ 推送完成，编译完成！二进制文件: ./netcfg"
echo "请将 netcfg 手动上传到生产服务器。"