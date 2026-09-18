# 智能助手会话删除与保留策略

## 数据范围

智能助手会话表保存用户拥有的会话标题、滚动摘要、最近资料来源和最后活动时间。消息表保存用户与助手消息、受限工具结果、引用 JSON、Token 数量和创建时间。这些数据用于恢复多轮上下文，不会跨用户共享。

运行审计与会话正文分开存储。审计只保存请求状态、角色、耗时、Token 汇总、费用估算和隐私安全的工具投影，不保存用户提示词、助手回答、授权头或原始敏感工具参数。

## 用户主动删除

`DELETE /api/agent/conversations/:session_id` 只按当前登录用户和会话句柄匹配。缺失、重复删除或属于其他用户的句柄返回相同的幂等成功响应，不暴露会话是否存在。

删除是永久且不可恢复的：

- 会话标题、摘要、来源记忆和全部消息通过级联删除立即清除；
- 相关运行审计的 `conversation_id` 和 `session_id` 被置空，审计统计继续按合规与运维需要保留；
- 系统仅保留当前用户 ID、会话句柄的 SHA-256 摘要、删除原因和删除时间组成的墓碑；墓碑不含消息或原始会话句柄，用于拒绝旧句柄重新创建；
- 删除后读取返回不存在，使用旧句柄继续发送消息返回 HTTP 410。

前端在删除前明确展示不可恢复提示。服务端仍独立执行所有者校验，不能依赖前端隐藏按钮。

## 自动保留期清理

`CSA_AGENT_CONVERSATION_RETENTION_DAYS` 配置按 `last_active_at` 计算的保留天数，范围为 1—3650。默认值 `0` 表示完全禁用自动清理，应用服务本身不会启动后台破坏性任务。

清理命令适合由操作系统定时任务、Kubernetes CronJob 或受控运维流程调用。默认只预览：

```bash
go run -buildvcs=false ./cmd/conversation_cleanup
```

输出中的 `dry_run=true`、`eligible` 和 `deleted=0` 用于审核影响范围。确认后必须显式提供 `-execute`：

```bash
go run -buildvcs=false ./cmd/conversation_cleanup -execute -batch-size=100
```

Docker 环境可使用同一镜像中的命令：

```bash
docker compose --env-file .env -f deployments/docker-compose.yml run --rm server ./conversation-cleanup
docker compose --env-file .env -f deployments/docker-compose.yml run --rm server ./conversation-cleanup -execute -batch-size=100
```

每批最多 1000 条。执行时会锁定候选会话，跳过其他清理进程已锁定的记录；同批完成墓碑写入、审计解绑和会话删除，任一步失败都会回滚该批。

## 运维建议

1. 首次启用保留策略时先运行 dry-run 并记录 `before` 与 `eligible`。
2. 在低峰期执行小批次清理，观察数据库延迟和错误日志。
3. 不要手工删除墓碑，否则旧客户端可能用已删除句柄创建空白新会话。
4. 运行迁移 036 后再启用清理命令；该迁移增加墓碑表和全局保留期扫描索引。
