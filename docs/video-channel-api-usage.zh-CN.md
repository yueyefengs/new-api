# 视频任务 API 使用说明

本文只保留客户端接入 `newapi` 视频任务时必须知道的信息：

- 调哪个接口创建任务
- 用哪个 ID 查询任务
- 任务完成后如何取结果

适用渠道：

- `DoubaoVideo`
- `Chengmeng`

## 核心原则

1. 客户端始终请求 `newapi`，不要直接请求上游渠道。
2. 创建成功后，客户端保存的是 `newapi` 返回的 `task_id`，不是上游的 `task_no`。
3. 查询任务状态时，继续请求 `newapi` 的查询接口。

这点尤其重要：

- `Chengmeng` 上游状态查询是 `GET /api/tasks/:taskNo`
- 但客户端不应该直接调用这个接口
- 客户端应该调用 `newapi` 的 `GET /v1/videos/:task_id` 或 `GET /v1/video/generations/:task_id`

## 推荐接口

推荐优先使用 OpenAI 兼容视频接口：

- 创建任务：`POST /v1/videos`
- 查询任务：`GET /v1/videos/{task_id}`

兼容旧路径：

- 创建任务：`POST /v1/video/generations`
- 查询任务：`GET /v1/video/generations/{task_id}`

任务成功后，如果需要通过网关取内容，还可以使用：

- 获取结果内容：`GET /v1/videos/{task_id}/content`

## 完整流程

### 1. 创建任务

请求：

```bash
curl -X POST "http://localhost:3000/v1/videos" \
  -H "Authorization: Bearer ${NEWAPI_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0",
    "prompt": "一辆复古列车穿越秋日山谷，电影感镜头，真实光影。",
    "duration": 5,
    "images": [],
    "metadata": {
      "orientation": "landscape",
      "size": "720"
    }
  }'
```

创建成功后，响应里会返回 `task_id`，例如：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx"
}
```

客户端必须保存这个 `task_id`，后续查询都用它。

### 2. 轮询任务状态

请求：

```bash
curl -X GET "http://localhost:3000/v1/videos/task_xxx" \
  -H "Authorization: Bearer ${NEWAPI_KEY}"
```

建议客户端每 `3-5` 秒轮询一次，直到任务进入终态。

常见终态：

- 成功：`SUCCESS` 或完成态
- 失败：`FAILURE`

不要拿上游返回的 `task_no` 查询 `newapi`。`newapi` 查询接口只认自己的 `task_id`。

### 3. 读取结果

任务成功后，通常可以从查询结果中拿到：

- `result_url`
- 或其他结果字段

如果你希望通过 `newapi` 代理拿内容，可以继续请求：

```bash
curl -X GET "http://localhost:3000/v1/videos/task_xxx/content" \
  -H "Authorization: Bearer ${NEWAPI_KEY}"
```

## 查询结果里重点关注的字段

客户端通常只需要关心下面几个字段：

- `task_id`：`newapi` 公开任务 ID
- `status`：当前任务状态
- `result_url`：任务成功后的结果地址
- `fail_reason`：任务失败原因
- `progress`：进度

## Chengmeng 调用要点

如果当前实际命中的是 `Chengmeng`：

1. 创建时仍然请求 `newapi`
2. `newapi` 会向上游提交任务并保存上游 `task_no`
3. 后台轮询和状态回查由 `newapi` 负责
4. 客户端只需要拿 `newapi task_id` 查询

也就是说，客户端不用感知 `Chengmeng` 的上游查询接口细节。

## 最小接入建议

推荐你按这个最小闭环接入，不要一开始把所有可选字段都加上：

1. 先用最简单的 `prompt + duration` 创建任务
2. 成功后保存 `task_id`
3. 每 `3-5` 秒轮询 `GET /v1/videos/{task_id}`
4. 成功后读取 `result_url` 或请求 `/v1/videos/{task_id}/content`
5. 失败时读取 `fail_reason`

如果你需要管理员强制指定具体渠道，才在 token 末尾追加 `-<channel_id>`。正常业务代码建议优先使用模型路由，不要把渠道 ID 写死在客户端。
