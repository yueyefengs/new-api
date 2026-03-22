# new-api 工作流说明

## Remote 配置

```bash
origin   → git@github.com:yueyefengs/new-api.git     (你的 fork)
upstream → git@github.com:QuantumNous/new-api.git     (官方仓库)
```

## 分支策略

```
upstream/main (官方) ─────────────────────────────►
       │
       ▼
origin/main (你的 main) ──────────────────────────►  ← 保持干净，只同步官方
       │
       ├── feature/自定义功能1 ───────────────────►  ← 独立功能分支
       │
       └── feature/自定义功能2 ───────────────────►  ← 独立功能分支
```

## 日常操作

### 同步官方更新
```bash
./sync-upstream.sh
```

### 创建新功能分支
```bash
# 从 main 创建新分支
git checkout main
git checkout -b feature/我的功能

# 开发完成后推送
git push origin feature/我的功能
```

### 更新功能分支
```bash
# 切换到功能分支
git checkout feature/我的功能

# 合并最新官方代码
git merge main

# 或使用 rebase（保持历史整洁）
git rebase main

# 推送
git push origin feature/我的功能
```

## Railway 部署

Railway 当前连接的是 `yueyefengs/new-api` 的 `main` 分支。

**如果要部署自定义功能：**
1. 创建功能分支
2. 在 Railway Settings → Source → Branch 中选择分支
3. 或合并功能分支到 main 后自动部署

## 注意事项

1. **main 分支保持干净** - 只同步官方，不直接修改
2. **功能独立分支** - 每个自定义功能独立分支
3. **定期同步** - 每周同步一次上游
4. **配置优于代码** - 优先使用环境变量配置
