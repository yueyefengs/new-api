#!/bin/bash
# sync-upstream.sh - 同步上游官方仓库更新
# 用法: ./sync-upstream.sh

set -e

echo "🔄 同步 new-api 上游更新..."
echo ""

cd ~/code/new-api

# 1. 获取上游最新代码
echo "📥 获取上游最新代码..."
git fetch upstream

# 2. 切换到 main 分支
echo "🔀 切换到 main 分支..."
git checkout main

# 3. 合并上游更新（快进合并）
echo "🔀 合并上游更新..."
git merge upstream/main --ff-only

# 4. 推送到 fork
echo "📤 推送到 fork..."
git push origin main

echo ""
echo "✅ 上游同步完成！"
echo ""
echo "📋 当前状态："
git log --oneline -3
echo ""
echo "💡 如有功能分支，请运行："
echo "   git checkout feature/xxx && git merge main"
