-- Ranking + lookup indexes for the hiscore read API (modules/hiscore).
-- Spec: docs/superpowers/specs/2026-08-19-hiscore-api-design.md
--
-- goscape extension over TS: Engine-TS has no hiscore serving endpoint,
-- so its prisma schemas declare no such indexes. Purely additive — no
-- behavioural change to the write path in modules/login.
--
-- The *_rank indexes match the API's total ordering exactly
-- (value DESC, date ASC, account_id ASC) so leaderboard pages and the
-- rank COUNT are served by an index range scan rather than a sort.
--
-- The *_account indexes serve the player-card lookup: the existing PK is
-- (profile, type, account_id), which cannot serve a query that knows
-- profile + account_id but not type.

CREATE INDEX idx_hiscore_rank
    ON hiscore (profile, type, value DESC, date ASC, account_id ASC);

CREATE INDEX idx_hiscore_account
    ON hiscore (profile, account_id);

CREATE INDEX idx_hiscore_large_rank
    ON hiscore_large (profile, type, value DESC, date ASC, account_id ASC);

CREATE INDEX idx_hiscore_large_account
    ON hiscore_large (profile, account_id);
