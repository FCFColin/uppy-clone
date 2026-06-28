# ADR-003: Magic Link 认证而非传统密码

## 状态

已接受

## 日期

2025-01

## 上下文

游戏需要低摩擦的用户认证。传统密码注册流程流失率高。

## 决策

实现两种认证方式：
1. Quick Play: 无需注册，自动生成临时账号，JWT 存入 HttpOnly cookie
2. Magic Link: 输入邮箱，发送登录链接，点击即登录

## 后果

**正面**
- Quick Play 零摩擦，降低流失率
- Magic Link 无需存储密码，消除密码泄露风险
- 符合 OAuth2/OIDC 趋势（无密码认证）

**负面**
- 依赖邮件服务 (Resend)，邮件延迟影响体验
- 无法离线认证（Magic Link 需要网络）
