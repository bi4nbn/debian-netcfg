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

echo -e "\n========== 拉取远程最新避免冲突 =========="
git pull origin main --rebase

echo -e "\n========== 推送到GitHub main分支 =========="
git push origin main

echo -e "\n✅ 推送完成！"