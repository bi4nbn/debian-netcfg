#!/bin/bash
set -e

echo "========== 查看本地变更 =========="
git status

echo -e "\n========== 全部加入暂存区（含二进制netcfg） =========="
git add .

read -p "请输入本次提交备注: " commitMsg
if [ -z "$commitMsg" ];then
    commitMsg="完整更新源码+编译二进制，修复网关计算、Bond默认LACP、下发layer3+4参数"
fi

echo -e "\n========== 提交变更 =========="
git commit -m "$commitMsg"

# 获取当前标签
latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [ -z "$latest_tag" ]; then
    latest_tag="v0.0"
fi
echo "当前最新标签: $latest_tag"

# 检查自最新标签以来的提交数
commit_count=$(git rev-list --count $latest_tag..HEAD 2>/dev/null || echo 0)
if [ "$commit_count" -eq 0 ]; then
    echo "没有新的提交，无需打新标签。"
else
    echo "自 $latest_tag 以来有 $commit_count 个新提交。"
    read -p "是否要打新标签？(y/n): " answer
    if [[ "$answer" =~ ^[Yy]$ ]]; then
        # 自动建议下一个版本号（递增最后一位，简单处理）
        current_version=${latest_tag#v}
        IFS='.' read -r -a version_parts <<< "$current_version"
        major=${version_parts[0]}
        minor=${version_parts[1]}
        patch=${version_parts[2]:-0}
        # 简单递增 patch
        new_patch=$((patch + 1))
        suggested_version="v$major.$minor.$new_patch"
        read -p "请输入新版本号 (默认 $suggested_version): " new_version
        if [ -z "$new_version" ]; then
            new_version=$suggested_version
        fi
        # 打标签
        git tag "$new_version"
        echo "已创建标签 $new_version"
    else
        echo "跳过打标签。"
    fi
fi

echo -e "\n========== 拉取远程最新避免冲突 =========="
git pull origin main --rebase

echo -e "\n========== 推送到GitHub main分支 =========="
git push origin main

# 如果有新标签，也推送标签
if [ -n "$new_version" ]; then
    echo "推送标签 $new_version ..."
    git push origin "$new_version"
fi

echo -e "\n✅ 推送完成！"