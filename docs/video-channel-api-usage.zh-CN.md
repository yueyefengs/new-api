# 视频渠道 API 使用说明

本文说明如何通过统一网关路由调用两个视频渠道：

- `DoubaoVideo`
- `Chengmeng`

统一接口：

- 提交任务：`POST /v1/video/generations`
- 查询任务：`GET /v1/video/generations/{task_id}`

网关地址示例：

- `http://localhost:3000`

## 重要提醒

- 请不要把真实密钥、真实渠道 ID、真实上游素材链接硬编码到仓库文件里。
- 视频生成通常会触发真实上游调用与计费，联调前请先确认额度、计费策略和渠道状态。
- 建议先用最短时长、最简单提示词做冒烟测试，把“路由是否正确”和“画质/成本是否满意”分开验证，不要一次把问题混在一起。
- 如果你要使用“指定渠道”方式，`-<channel_id>` 里的 `channel_id` 是管理员后台中的实际渠道记录 ID，不是渠道类型常量。比如 `158` 在很多上下文里表示的是 `Chengmeng` 的渠道类型，不等于你后台里的具体渠道记录 ID。

## 两种稳定路由方式

### 1. 基于模型名路由

这是默认推荐方式。

- `DoubaoVideo`：使用模型 `doubao-seedance-2-0`
- `Chengmeng`：也统一使用模型 `doubao-seedance-2-0`

只要你的渠道配置与模型映射保持一致，提交时直接带对应模型名即可稳定命中目标渠道。

### 2. 管理员专用：通过 token 后缀强制指定渠道

可在 token 末尾追加 `-<channel_id>`：

- 原 token：`sk-your-token`
- 指定渠道后：`sk-your-token-123`

原因是当前认证逻辑会先去掉 `sk-`，再按 `-` 分割，首段作为真实 token，后缀可用于附加渠道选择信息。

示例：

```bash
Authorization: Bearer sk-your-token-123
```

注意：

- 这是管理员或调试场景用法，不建议写死在正式业务代码里。
- 查询任务 `GET /v1/video/generations/{task_id}` 通常不需要再指定渠道，因为网关会按已保存的任务记录回查。

## DoubaoVideo 用法

`DoubaoVideo` 现在支持在 `/v1/video/generations` 上直接提交官方风格请求体，字段包括：

- `model`
- `content`
- `generate_audio`
- `ratio`
- `duration`
- `watermark`

### 请求示例

```bash
curl -X POST "http://localhost:3000/v1/video/generations" \
  -H "Authorization: Bearer ${NEWAPI_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0",
    "content": [
      {
        "type": "text",
        "text": "一只机械鲸鱼跃出海面，电影感镜头，真实光影。"
      }
    ],
    "generate_audio": false,
    "ratio": "16:9",
    "duration": 5,
    "watermark": false
  }'
```

### 指定 DoubaoVideo 具体渠道示例

```bash
curl -X POST "http://localhost:3000/v1/video/generations" \
  -H "Authorization: Bearer ${NEWAPI_KEY}-${DOUBAO_CHANNEL_ID}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0",
    "content": [
      {
        "type": "text",
        "text": "黄昏城市上空，一架无人机穿过霓虹雨幕。"
      }
    ],
    "generate_audio": false,
    "ratio": "16:9",
    "duration": 5,
    "watermark": false
  }'
```

## Chengmeng 用法

`Chengmeng` 建议使用简化请求体，而不是官方风格请求体。

推荐字段：

- `model`: 固定用 `doubao-seedance-2-0`
- `prompt`
- `duration` 或 `seconds`
- `images`
- `metadata.videos`
- `metadata.orientation`
- `metadata.size`

### 请求示例

```bash
curl -X POST "http://localhost:3000/v1/video/generations" \
  -H "Authorization: Bearer ${NEWAPI_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0",
    "prompt": "清晨薄雾中的湖边小屋，镜头缓慢推进，光线柔和。",
    "duration": 5,
    "images": [],
    "metadata": {
      "videos": [],
      "orientation": "landscape",
      "size": "720"
    }
  }'
```

### 指定 Chengmeng 具体渠道示例

```bash
curl -X POST "http://localhost:3000/v1/video/generations" \
  -H "Authorization: Bearer ${NEWAPI_KEY}-${CHENGMENG_CHANNEL_ID}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0",
    "prompt": "一辆复古列车穿越秋日山谷，远景到中景切换。",
    "duration": 5,
    "images": [],
    "metadata": {
      "videos": [],
      "orientation": "landscape",
      "size": "720"
    }
  }'
```

## Chengmeng 不支持的字段

`Chengmeng` 不支持：

- `audio_url`
- `generate_audio`

如果你把这两个字段通过官方风格请求体传给 `Chengmeng`，网关会返回明确的“不支持”错误，而不是静默忽略。

典型错误语义：

- `audio_url is not supported by Chengmeng`
- `generate_audio is not supported by Chengmeng`

所以不要把 `DoubaoVideo` 的官方风格音频参数直接照搬到 `Chengmeng`。

## 查询任务状态

提交成功后，响应里通常会返回网关公开任务 ID，例如：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx"
}
```

拿这个 `task_id` 查询：

```bash
curl -X GET "http://localhost:3000/v1/video/generations/task_xxx" \
  -H "Authorization: Bearer ${NEWAPI_KEY}"
```

说明：

- 这里使用的是网关公开任务 ID，不是上游原始任务 ID。
- 查询时通常不需要再附加 `-<channel_id>`。
- 常见状态一般包括：`queued`、`in_progress`、`completed`、`failed`。

## 最小联调建议

建议按下面顺序验证，避免排查方向混乱：

1. 先用模型路由测一次：
   - `doubao-seedance-2-0`
2. 再用 token 后缀强制指定渠道测一次，确认管理员定向路由正常。
3. 最后再加参考图、参考视频、音频相关参数，不要一开始就把所有可选项都塞进去。

## 配套脚本

仓库内可配合以下脚本进行快速联调：

- `scripts/test_video_channels.sh`

该脚本会：

- 提交一个 `DoubaoVideo` 请求
- 提交一个 `Chengmeng` 请求
- 如果响应里带有任务 ID，则自动轮询任务状态
- 明确提示真实调用可能产生上游费用
