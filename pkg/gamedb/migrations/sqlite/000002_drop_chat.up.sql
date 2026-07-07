-- Chat is Kafka-only: public_chat/private_chat are retired (documented
-- TS divergence — TS persists chat to these tables; goscape emits
-- PublicChatEvent/PrivateChatEvent telemetry instead; spec
-- docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md).
-- Destroys existing chat rows by design (approved: no backfill).
DROP TABLE IF EXISTS public_chat;
DROP TABLE IF EXISTS private_chat;
