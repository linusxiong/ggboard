-- Fake data for the legacy fixture (docs/IMPORT_XBOARD.md §9). Every address is on example.com; every key,
-- token and secret is an obvious placeholder. All passwords are "password123".

INSERT INTO "migrations" ("migration", "batch") VALUES
('2019_08_19_000000_create_failed_jobs_table', 1),
('2023_08_07_205816_create_v2_commission_log_table', 1),
('2023_08_07_205816_create_v2_coupon_table', 1),
('2023_08_07_205816_create_v2_invite_code_table', 1),
('2023_08_07_205816_create_v2_knowledge_table', 1),
('2023_08_07_205816_create_v2_log_table', 1),
('2023_08_07_205816_create_v2_mail_log_table', 1),
('2023_08_07_205816_create_v2_notice_table', 1),
('2023_08_07_205816_create_v2_order_table', 1),
('2023_08_07_205816_create_v2_payment_table', 1),
('2023_08_07_205816_create_v2_plan_table', 1),
('2023_08_07_205816_create_v2_server_group_table', 1),
('2023_08_07_205816_create_v2_server_hysteria_table', 1),
('2023_08_07_205816_create_v2_server_route_table', 1),
('2023_08_07_205816_create_v2_server_shadowsocks_table', 1),
('2023_08_07_205816_create_v2_server_trojan_table', 1),
('2023_08_07_205816_create_v2_server_vless_table', 1),
('2023_08_07_205816_create_v2_server_vmess_table', 1),
('2023_08_07_205816_create_v2_stat_server_table', 1),
('2023_08_07_205816_create_v2_stat_table', 1),
('2023_08_07_205816_create_v2_stat_user_table', 1),
('2023_08_07_205816_create_v2_ticket_message_table', 1),
('2023_08_07_205816_create_v2_ticket_table', 1),
('2023_08_07_205816_create_v2_user_table', 1),
('2023_08_14_221234_create_v2_settings_table', 1),
('2023_09_04_190923_add_column_excludes_to_server_table', 1),
('2023_09_06_195956_add_column_ips_to_server_table', 1),
('2023_09_14_013244_add_column_alpn_to_server_hysteria_table', 1),
('2023_09_24_040317_add_column_network_and_network_settings_to_v2_server_trojan', 1),
('2023_09_29_044957_add_column_version_and_is_obfs_to_server_hysteria_table', 1),
('2023_11_19_205026_change_column_value_to_v2_settings_table', 1),
('2023_12_12_212239_add_index_to_v2_user_table', 1),
('2024_03_19_103149_modify_icon_column_to_v2_payment_table', 1);

INSERT INTO "v2_server_group" ("id", "name", "created_at", "updated_at") VALUES
(1, 'Group A', 1700000000, 1700000000),
(2, 'Group B', 1700000000, 1700000000),
(3, 'Group C', 1700000000, 1700000000);

-- route 1: match is a JSON object (PHP array_filter + json_encode); route 2: pre-JSON newline list
INSERT INTO "v2_server_route" ("id", "remarks", "match", "action", "action_value", "created_at", "updated_at") VALUES
(1, 'block ads', '{"0":"domain:ads.example.com","2":"domain:track.example.com"}', 'block', NULL, 1700000000, 1700000000),
(2, 'dns', 'geosite:netflix
domain:video.example.com', 'dns', '1.1.1.1', 1700000000, 1700000000);

-- plan 1: month 1000 + free onetime; plan 2: reset monthly on expiry day, capacity 0 = sold out (exceeded by 1 active user);
-- plan 3: unknown group, yearly reset on Jan 1; plan 4: never resets
INSERT INTO "v2_plan" ("id", "group_id", "transfer_enable", "name", "speed_limit", "show", "sort", "renew", "content", "month_price", "quarter_price", "half_year_price", "year_price", "two_year_price", "three_year_price", "onetime_price", "reset_price", "reset_traffic_method", "capacity_limit", "created_at", "updated_at") VALUES
(1, 1, 100, 'Basic', 0, 1, 1, 1, 'Basic plan', 1000, NULL, NULL, NULL, NULL, NULL, 0, NULL, NULL, NULL, 1700000000, 1700000000),
(2, 2, 500, 'Pro', 100, 0, 2, 1, NULL, 2000, NULL, NULL, 20000, NULL, NULL, NULL, 500, 1, 0, 1700000000, 1700000000),
(3, 9, 50, 'Orphan', NULL, 1, NULL, 0, 'no group', NULL, 2500, NULL, NULL, NULL, NULL, NULL, NULL, 3, NULL, 1700000000, 1700000000),
(4, 1, 10, 'Never reset', NULL, 1, 3, 1, '', 500, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 2, NULL, 1700000000, 1700000000);

-- expired_at 1801360800 = 2027-01-31 10:00 +08:00; 1600000000 is in the past
INSERT INTO "v2_user" ("id", "invite_user_id", "telegram_id", "email", "password", "password_algo", "password_salt", "balance", "discount", "commission_type", "commission_rate", "commission_balance", "t", "u", "d", "transfer_enable", "banned", "is_admin", "last_login_at", "is_staff", "last_login_ip", "uuid", "group_id", "plan_id", "speed_limit", "remind_expire", "remind_traffic", "token", "expired_at", "remarks", "created_at", "updated_at") VALUES
(1, NULL, 123456789, 'admin@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 1000, NULL, 0, NULL, 500, 0, 1024, 2048, 107374182400, 0, 1, 1700000300, 0, NULL, '00000000-0000-4000-8000-000000000001', 1, 1, NULL, 1, 1, '00000000000000000000000000000001', 1801360800, 'root', 1690000000, 1700000000),
(2, 1, NULL, 'md5@example.com', '482c811da5d5b4bc6d497ffa98491e38', 'md5', NULL, 0, 10, 1, 25, 0, 0, 0, 0, 536870912000, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000002', 2, 2, 100, 1, 0, '00000000000000000000000000000002', 1801360800, NULL, 1690000000, 1700000000),
(3, NULL, NULL, 'sha256@example.com', 'ef92b778bafe771e89245b89ecbc08a44a4e166c06659911881f383d4473e94f', 'sha256', NULL, 0, NULL, 2, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 1, NULL, '00000000-0000-4000-8000-000000000003', 1, 3, NULL, 0, 1, '00000000000000000000000000000003', 1801360800, NULL, 1690000000, 1700000000),
(4, NULL, NULL, 'md5salt@example.com', '1b84a6e599feeab25ace9f402aaa4858', 'md5salt', 'abc$12', 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000004', 1, 4, NULL, 1, 1, '00000000000000000000000000000004', NULL, NULL, 1690000000, 1700000000),
(5, NULL, NULL, 'unknown@example.com', 'cbfdac6008f9cab4083784cbd1874f76618d2a97', 'sha1', NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000005', NULL, NULL, NULL, 1, 1, '00000000000000000000000000000005', 0, NULL, 1690000000, 1700000000),
(6, NULL, NULL, 'expired0@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000006', 1, 1, NULL, 1, 1, '00000000000000000000000000000006', 0, NULL, 1690000000, 1700000000),
(7, NULL, NULL, 'past@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000007', 2, 2, NULL, 1, 1, '00000000000000000000000000000007', 1600000000, NULL, 1590000000, 1700000000),
(8, 77, NULL, 'deletedplan@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000008', 1, 99, NULL, 1, 1, '00000000000000000000000000000008', 1801360800, NULL, 1690000000, 1700000000),
(9, 9, NULL, 'selfinvite@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', '', '', 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000009', NULL, NULL, NULL, 1, 1, '00000000000000000000000000000001', NULL, NULL, 1690000000, 1700000000),
(10, NULL, NULL, 'Mixed@Example.com ', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000010', NULL, NULL, NULL, NULL, NULL, '00000000000000000000000000000010', NULL, NULL, 1690000000, 1700000000),
(11, NULL, NULL, 'dupuuid@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000002', NULL, NULL, NULL, 1, 1, '00000000000000000000000000000011', NULL, NULL, 1690000000, 1700000000),
(12, NULL, NULL, 'banned@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 5, NULL, 0, 0, -5, 10, 0, 1, 0, NULL, 0, NULL, '00000000-0000-4000-8000-000000000012', NULL, NULL, NULL, 1, 1, '00000000000000000000000000000012', NULL, NULL, 1690000000, 1700000000),
(13, NULL, NULL, 'A@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, 1700000100, 0, NULL, '00000000-0000-4000-8000-000000000013', NULL, NULL, NULL, 1, 1, '00000000000000000000000000000013', NULL, NULL, 1690000000, 1690000000),
(14, NULL, NULL, 'a@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 0, 1700000200, 0, NULL, '00000000-0000-4000-8000-000000000014', NULL, NULL, NULL, 1, 1, '00000000000000000000000000000014', NULL, NULL, 1690000000, 1690000000);

INSERT INTO "v2_invite_code" ("id", "user_id", "code", "status", "pv", "created_at", "updated_at") VALUES
(1, 1, 'INVITE01', 0, 3, 1700000000, 1700000000),
(2, 2, 'invite01', 1, 0, 1700000000, 1700000000),
(3, 99, 'ORPHAN99', 0, 0, 1700000000, 1700000000);

INSERT INTO "v2_commission_log" ("id", "invite_user_id", "user_id", "trade_no", "order_amount", "get_amount", "created_at", "updated_at") VALUES
(1, 1, 2, '2024010112000012345678', 1000, 100, 1700000000, 1700000000),
(2, 77, 2, '2024010112000012345679', 2000, 200, 1700000000, 1700000000);

INSERT INTO "v2_coupon" ("id", "code", "name", "type", "value", "show", "limit_use", "limit_use_with_user", "limit_plan_ids", "limit_period", "started_at", "ended_at", "created_at", "updated_at") VALUES
(1, 'WELCOME', 'Welcome', 1, 500, 1, 10, 1, '["1","2","99"]', '["month_price","year_price","bogus"]', 1700000000, 1900000000, 1700000000, 1700000000),
(2, 'welcome', 'Duplicate', 1, 100, 0, NULL, NULL, NULL, NULL, 1700000000, 1900000000, 1700000000, 1700000000),
(3, 'THIS-COUPON-CODE-IS-MUCH-TOO-LONG-FOR-GGBOARD-0123456789-0123456789-X', 'Long', 2, 10, 0, NULL, NULL, NULL, NULL, 1700000000, 1900000000, 1700000000, 1700000000),
(4, 'PERCENT', 'Percent', 2, 20, 0, NULL, NULL, NULL, NULL, 1700000000, 1900000000, 1700000000, 1700000000);

INSERT INTO "v2_payment" ("id", "uuid", "payment", "name", "icon", "config", "notify_domain", "handling_fee_fixed", "handling_fee_percent", "enable", "sort", "created_at", "updated_at") VALUES
(1, 'AbCd1234', 'EPay', 'Alipay (EPay)', NULL, '{"url":"https://pay.example.com/","pid":"1001","key":"epay-example-key","type":"alipay"}', NULL, 100, 1.5, 1, 1, 1700000000, 1700000000),
(2, 'EfGh5678', 'EpusdtPay', 'USDT', 'usdt.png', '{"epusdt_pay_url":"https://usdt.example.com/","epusdt_pay_apitoken":"usdt-example-token","currency_types":""}', '', NULL, NULL, 1, 2, 1700000000, 1700000000),
(3, 'IjKl9012', 'AlipayF2F', 'Alipay F2F', NULL, '{"app_id":"2021000000000000","private_key":"EXAMPLE-PRIVATE-KEY","public_key":"EXAMPLE-PUBLIC-KEY","product_name":"VIP"}', NULL, NULL, NULL, 1, 3, 1700000000, 1700000000),
(4, 'MnOp3456', 'StripeALLInOne', 'Card', NULL, '{"currency":"usd","stripe_sk_live":"sk_test_example","stripe_webhook_key":"whsec_example","description":"x","payment_method":"cards"}', 'https://pay-notify.example.com', NULL, NULL, 1, 4, 1700000000, 1700000000),
(5, 'QrSt7890', 'Coinbase', 'Crypto', NULL, '{"coinbase_url":"https://api.commerce.coinbase.com","coinbase_api_key":"cb-example-key","coinbase_webhook_key":"cb-example-hook"}', NULL, NULL, NULL, 1, 5, 1700000000, 1700000000),
(6, 'UvWx1234', 'BTCPay', 'Bitcoin', NULL, '{"btcpay_url":"https://btcpay.example.com/","btcpay_storeId":"store1","btcpay_api_key":"btc-example-key","btcpay_webhook_key":"btc-example-hook"}', NULL, NULL, NULL, 1, 6, 1700000000, 1700000000),
(7, 'YzAb5678', 'MGate', 'MGate', NULL, '{"mgate_url":"https://mgate.example.com","mgate_app_id":"app","mgate_app_secret":"mgate-example-secret"}', NULL, NULL, NULL, 1, 7, 1700000000, 1700000000);

-- ticket 1: staff (user 1) replied last; ticket 2: the user replied last; ticket 3: no messages (legacy reply_status 0 = staff replied);
-- ticket 4: user 99 does not exist
INSERT INTO "v2_ticket" ("id", "user_id", "subject", "level", "status", "reply_status", "created_at", "updated_at") VALUES
(1, 2, 'Staff replied last', 0, 0, 0, 1700000000, 1700000100),
(2, 3, 'User replied last', 1, 0, 1, 1700000000, 1700000100),
(3, 2, 'No messages', 2, 1, 0, 1700000000, 1700000000),
(4, 99, 'Orphan', 0, 0, 1, 1700000000, 1700000000);

INSERT INTO "v2_ticket_message" ("id", "user_id", "ticket_id", "message", "created_at", "updated_at") VALUES
(1, 2, 1, 'My node is down', 1700000000, 1700000000),
(2, 1, 1, 'Fixed, please retry', 1700000100, 1700000100),
(3, 1, 2, 'Anything else?', 1700000050, 1700000050),
(4, 3, 2, 'Yes, one more question', 1700000100, 1700000100),
(5, 99, 4, 'orphan message', 1700000000, 1700000000);

INSERT INTO "v2_notice" ("id", "title", "content", "show", "img_url", "tags", "created_at", "updated_at") VALUES
(1, 'Old', 'old notice', 1, 'https://img.example.com/a.png', '["news","update"]', 1700000000, 1700000000),
(2, 'New', 'new notice', 1, NULL, NULL, 1700100000, 1700100000),
(3, 'Same time, higher id', 'third', 0, NULL, 'maintenance', 1700100000, 1700100000);

INSERT INTO "v2_knowledge" ("id", "language", "category", "title", "body", "sort", "show", "created_at", "updated_at") VALUES
(1, 'zh-CN', 'Guide', 'Windows', 'Download the client from https://example.com', 1, 1, 1700000000, 1700000000),
(2, 'en-US', 'Guide', 'Members', 'Hello <!--access start-->members only<!--access end-->', NULL, 0, 1700000000, 1700000000);

-- settings: app_name appears twice (the highest id wins); reset_traffic_method is absent
INSERT INTO "v2_settings" ("id", "group", "type", "name", "value", "created_at", "updated_at") VALUES
(1, NULL, NULL, 'app_name', 'Old Name', '2023-11-15 06:13:20', '2023-11-15 06:13:20'),
(2, NULL, NULL, 'app_url', 'https://board.example.com', NULL, NULL),
(3, NULL, NULL, 'stop_register', '', NULL, NULL),
(4, NULL, NULL, 'email_verify', '1', NULL, NULL),
(5, NULL, NULL, 'invite_force', 'true', NULL, NULL),
(6, NULL, NULL, 'subscribe_url', 'https://a.example.com,https://*.example.com', NULL, NULL),
(7, NULL, NULL, 'server_token', 'xboard-example-server-token', NULL, NULL),
(8, NULL, NULL, 'try_out_plan_id', '99', NULL, NULL),
(9, NULL, NULL, 'recaptcha_enable', '1', NULL, NULL),
(10, NULL, NULL, 'recaptcha_site_key', 'recaptcha-example-site', NULL, NULL),
(11, NULL, NULL, 'recaptcha_key', 'recaptcha-example-secret', NULL, NULL),
(12, NULL, NULL, 'email_host', 'smtp.example.com', NULL, NULL),
(13, NULL, NULL, 'email_port', '465', NULL, NULL),
(14, NULL, NULL, 'email_encryption', '', NULL, NULL),
(15, NULL, NULL, 'invite_commission', '12.5', NULL, NULL),
(16, NULL, NULL, 'frontend_theme', 'v2board', NULL, NULL),
(17, NULL, NULL, 'telegram_bot_enable', '0', NULL, NULL),
(18, NULL, NULL, 'some_unknown_key', 'x', NULL, NULL),
(19, NULL, NULL, 'currency', 'CNY', NULL, NULL),
(20, NULL, NULL, 'email_whitelist_enable', '1', NULL, NULL),
(21, NULL, NULL, 'app_name', 'Example Board', NULL, NULL),
(22, NULL, NULL, 'plan_change_enable', '0', NULL, NULL),
(23, NULL, NULL, 'server_push_interval', '30', NULL, NULL);

-- nodes: ids collide across tables (vmess #1 and #2 collide with shadowsocks #1 and #2); max legacy id is 10
INSERT INTO "v2_server_shadowsocks" ("id", "group_id", "route_id", "parent_id", "tags", "name", "rate", "host", "port", "server_port", "cipher", "obfs", "obfs_settings", "show", "sort", "created_at", "updated_at", "excludes", "ips") VALUES
(1, '["1","2"]', '["1","2"]', NULL, '["HK","SS"]', 'SS-2022', '1', 'ss.example.com', '443', 443, '2022-blake3-aes-128-gcm', NULL, NULL, 1, 1, 1700000000, 1700000000, NULL, NULL),
(2, '[1]', NULL, 1, NULL, 'SS-relay', '1.5', 'relay.example.com', '8443', 443, '2022-blake3-aes-128-gcm', NULL, NULL, 1, 2, 1700001234, 1700001234, NULL, NULL),
(3, '{"0":"1","2":"3"}', '[]', NULL, NULL, 'SS-obfs', '0.5', 'obfs.example.com', '8388', 8388, 'aes-128-gcm', 'http', '{"path":"/","host":"cdn.example.com"}', 1, 3, 1700000000, 1700000000, '["CN"]', NULL),
(4, '1,2', NULL, NULL, NULL, 'SS-text-groups', 'abc', 'text.example.com', '8389', 8389, 'aes-192-gcm', NULL, NULL, 0, NULL, 1700000000, 1700000000, NULL, NULL),
(5, '', NULL, NULL, NULL, 'SS-no-groups', '1', 'empty.example.com', '8390', 8390, 'chacha20-ietf-poly1305', NULL, NULL, 1, NULL, 1700000000, 1700000000, NULL, '1.2.3.4');

INSERT INTO "v2_server_vmess" ("id", "group_id", "route_id", "name", "parent_id", "host", "port", "server_port", "tls", "tags", "rate", "network", "rules", "networkSettings", "tlsSettings", "ruleSettings", "dnsSettings", "show", "sort", "created_at", "updated_at", "excludes", "ips") VALUES
(1, '[1]', '["1"]', 'VM-1', NULL, 'vm.example.com', '443', 10086, 1, NULL, '1', 'ws', '["geosite:cn"]', '{"path":"/ws","headers":{"Host":"vm.example.com"}}', '{"serverName":"vm.example.com","allowInsecure":"1"}', NULL, NULL, 1, 1, 1700000500, 1700000500, NULL, NULL),
(2, '[1]', NULL, 'VM-1-relay', 1, 'vm-relay.example.com', '443', 10086, 0, NULL, '1', 'tcp', NULL, NULL, NULL, NULL, NULL, 1, 2, 1700000600, 1700000600, NULL, NULL);

INSERT INTO "v2_server_trojan" ("id", "group_id", "route_id", "parent_id", "tags", "name", "rate", "host", "port", "server_port", "allow_insecure", "server_name", "show", "sort", "created_at", "updated_at", "excludes", "ips", "network", "networkSettings") VALUES
(6, '"1,2"', NULL, NULL, NULL, 'TJ', '2', 'tj.example.com', '443', 443, 0, 'tj.example.com', 1, 1, 1700000000, 1700000000, NULL, NULL, 'ws', '{"path":"/tj"}');

INSERT INTO "v2_server_hysteria" ("id", "group_id", "route_id", "name", "parent_id", "host", "port", "server_port", "tags", "rate", "show", "sort", "up_mbps", "down_mbps", "server_name", "insecure", "created_at", "updated_at", "excludes", "ips", "alpn", "version", "is_obfs") VALUES
(7, '[1]', NULL, 'HY1', NULL, 'hy.example.com', '20000-30000', 443, NULL, '1', 1, 1, 100, 200, 'hy.example.com', 1, 1700000000, 1700000000, NULL, NULL, 3, 1, 1),
(8, '', NULL, 'HY-v0', NULL, 'hy0.example.com', '443', 443, NULL, '1', 1, 2, 50, 50, NULL, 0, 1700000000, 1700000000, NULL, NULL, 0, 0, 0);

INSERT INTO "v2_server_vless" ("id", "group_id", "route_id", "name", "parent_id", "host", "port", "server_port", "tls", "tls_settings", "flow", "network", "network_settings", "tags", "rate", "show", "sort", "created_at", "updated_at", "excludes", "ips") VALUES
(9, '[1]', NULL, 'VL-reality', NULL, 'vl.example.com', 443, 443, 2, '{"server_name":"www.example.com","server_port":"443","public_key":"example-public-key","private_key":"example-private-key","short_id":"abcd","allow_insecure":0}', 'xtls-rprx-vision', 'tcp', NULL, '["Reality"]', '1', 1, 1, 1700000000, 1700000000, NULL, NULL),
(10, '["2"]', NULL, 'VL-kcp', NULL, 'kcp.example.com', 443, 443, 0, NULL, NULL, 'kcp', '{"header":{"type":"none"}}', NULL, '1', 1, 2, 1700000000, 1700000000, NULL, NULL);

-- stats: 1729440000 = 2024-10-21 00:00 +08:00; 1720000000 is outside a 90-day window ending 2024-10-22
INSERT INTO "v2_stat_server" ("id", "server_id", "server_type", "u", "d", "record_type", "record_at", "created_at", "updated_at") VALUES
(1, 1, 'vmess', 100, 200, 'd', 1729440000, 1729440000, 1729440000),
(2, 1, 'shadowsocks', 10, 20, 'd', 1729440000, 1729440000, 1729440000),
(3, 99, 'vmess', 1, 1, 'd', 1729440000, 1729440000, 1729440000),
(4, 2, 'v2ray', 5, 5, 'd', 1729440000, 1729440000, 1729440000),
(5, 1, 'vmess', 7, 7, 'd', 1720000000, 1720000000, 1720000000);

INSERT INTO "v2_stat_user" ("id", "user_id", "server_rate", "u", "d", "record_type", "record_at", "created_at", "updated_at") VALUES
(1, 1, 1.00, 1000, 2000, 'd', 1729440000, 1729440000, 1729440000),
(2, 1, 0.50, 400, 600, 'd', 1729440000, 1729440000, 1729440000),
(3, 99, 1.00, 1, 1, 'd', 1729440000, 1729440000, 1729440000),
(4, 1, 1.00, 9, 9, 'm', 1729440000, 1729440000, 1729440000);

INSERT INTO "v2_stat" ("id", "record_at", "record_type", "order_count", "order_total", "commission_count", "commission_total", "paid_count", "paid_total", "register_count", "invite_count", "transfer_used_total", "created_at", "updated_at") VALUES
(1, 1729440000, 'd', 1, 100, 0, 0, 1, 100, 1, 0, '1', 1729440000, 1729440000),
(2, 1729440000, 'd', 3, 3000, 1, 300, 2, 2000, 5, 2, '123456789', 1729440000, 1729440000);

INSERT INTO "v2_order" ("id", "invite_user_id", "user_id", "plan_id", "type", "period", "trade_no", "total_amount", "status", "commission_status", "commission_balance", "created_at", "updated_at") VALUES
(1, NULL, 2, 2, 1, 'month_price', '2024010112000012345678', 2000, 3, 2, 0, 1700000000, 1700000000);

INSERT INTO "v2_log" ("id", "title", "uri", "method", "created_at", "updated_at") VALUES
(1, 'example log', '/api/v1/example', 'GET', 1700000000, 1700000000);
