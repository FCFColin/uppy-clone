# ADR-022: 字段级 AES-256-GCM 加密 + HMAC email_hash 索引

## 状态: 已接受（部分落地）

## 上下文

项目章程（ADR-000）将 GDPR 合规作为一等公民。用户邮箱属于 PII，需要：
- 数据库泄露时邮箱不可直接读取
- 仍支持按邮箱查找用户（Magic Link 登录）
- 支持 GDPR 数据导出和硬删除
- 审计日志完整性独立于业务数据加密

Migration `000010_add_email_hash` 添加了 `email_hash` 列用于不可逆查找。

## 决策

采用 **双层策略**：

1. **存储加密**：用户邮箱等 PII 字段使用 AES-256-GCM 加密后存入 PostgreSQL
   - 密钥来自 `ENCRYPTION_KEY` 环境变量（64 hex chars = 32 bytes）
   - 每次加密生成随机 nonce（`crypto/aes.go:80-86`）
   - Nonce 前置到密文（`aes.go:122-123`）
   - 启动时 `crypto.InitFromEnv()` 强制校验（`aes.go:47-59`）

2. **查找索引**：邮箱的 HMAC-SHA256 哈希存入 `email_hash` 列
   - Magic Link 请求时计算 hash 查找用户，无需解密全表
   - Hash 不可逆，即使 `email_hash` 泄露也无法还原邮箱

3. **Magic Link token**：Redis 中存储 token 的 SHA-256 哈希（`magiclink.go:74-75`），非明文

## 后果

**正面**
- 数据库备份/泄露不直接暴露邮箱
- 查找性能通过 `email_hash` 索引保证
- AES-GCM 提供认证加密（防篡改）

**负面**
- `RotateKey()` 未实现（`aes.go:162-165`），密钥轮换无路径
- `EncryptEmailForStorage` 在 `encKey == nil` 时回退明文（`aes.go:146-149`），仅 dev 安全
- 加密字段无法直接 SQL `LIKE` 查询
- `AUDIT_SECRET` 当前可回退到 `JWT_SECRET`（`main.go:113`），审计完整性与签名密钥耦合

**放弃的替代方案**
- 全库透明数据加密（TDE）：依赖云厂商，不可移植
- 外部 KMS（Vault/GCP KMS）：增加运维复杂度，学习项目暂不引入
- 仅 bcrypt 哈希邮箱：无法还原，不支持数据导出
