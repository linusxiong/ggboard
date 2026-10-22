-- Fake data for the upstream fixture (docs/IMPORT_XBOARD.md §9). Every address is on example.com; every key,
-- token and secret is an obvious placeholder. All passwords are "password123".

INSERT INTO "migrations" ("migration", "batch") VALUES
('2019_08_19_000000_create_failed_jobs_table', 1),
('2019_12_14_000001_create_personal_access_tokens_table', 1),
('2023_03_19_000000_create_v2_tables', 1),
('2023_08_14_221234_create_v2_settings_table', 1),
('2023_09_04_190923_add_column_excludes_to_server_table', 1),
('2023_09_06_195956_add_column_ips_to_server_table', 1),
('2023_09_14_013244_add_column_alpn_to_server_hysteria_table', 1),
('2023_09_24_040317_add_column_network_and_network_settings_to_v2_server_trojan', 1),
('2023_09_29_044957_add_column_version_and_is_obfs_to_server_hysteria_table', 1),
('2023_11_19_205026_change_column_value_to_v2_settings_table', 1),
('2023_12_12_212239_add_index_to_v2_user_table', 1),
('2024_03_19_103149_modify_icon_column_to_v2_payment_table', 1),
('2025_01_01_130644_modify_commission_status_in_v2_order_table', 2),
('2025_01_04_optimize_plan_table', 2),
('2025_01_05_131425_create_v2_server_table', 2),
('2025_01_10_152139_add_device_limit_column', 2),
('2025_01_12_190315_add_sort_to_v2_notice_table', 2),
('2025_01_12_200936_modify_commission_status_in_v2_order_table', 2),
('2025_01_13_000000_convert_order_period_fields', 2),
('2025_01_15_000002_add_stat_performance_indexes', 2),
('2025_01_16_142320_add_updated_at_index_to_v2_order_table', 2),
('2025_01_18_140511_create_plugins_table', 2),
('2025_06_21_000001_optimize_v2_settings_table', 3),
('2025_06_21_000002_create_traffic_reset_logs_table', 3),
('2025_06_21_000003_add_traffic_reset_fields_to_users', 3),
('2025_07_01_081556_add_tags_to_v2_plan_table', 3),
('2025_07_01_122908_create_gift_card_tables', 3),
('2025_07_13_224539_add_column_rate_time_ranges_to_v2_server_table', 3),
('2025_07_26_000001_add_type_to_plugins_table', 3),
('2025_07_27_000001_create_v2_subscribe_templates_table', 3),
('2026_03_11_000001_replace_v2_log_with_admin_audit_log', 4),
('2026_03_11_000002_add_stat_user_record_at_index', 4),
('2026_03_15_060035_add_custom_config_and_cert_to_v2_server_table', 4),
('2026_03_28_161536_add_traffic_fields_to_servers', 4),
('2026_04_11_000001_add_machine_support', 4),
('2026_04_18_000001_add_network_to_machine_load_history', 4),
('2026_04_18_000001_make_server_enabled_nullable', 4),
('2026_04_19_000001_backfill_users_next_reset_at', 4),
('2026_04_19_235904_backfill_utls_for_legacy_servers', 4),
('2026_04_20_000001_create_mail_templates_table', 4),
('2026_04_21_000001_fix_ticket_reply_status', 4),
('2026_04_22_000001_normalize_trojan_tls_settings', 4),
('2026_05_30_000000_rename_refund_amount_to_surplus_credit', 5);

INSERT INTO "v2_server_group" ("id", "name", "created_at", "updated_at") VALUES
(1, 'Default', 1700000000, 1700000000),
(2, 'VIP', 1700000000, 1700000000);

-- route 2 uses an upstream-only action and is skipped
INSERT INTO "v2_server_route" ("id", "remarks", "match", "action", "action_value", "created_at", "updated_at") VALUES
(1, 'block ads', '["domain:ads.example.com","geosite:category-ads"]', 'block', NULL, 1700000000, 1700000000),
(2, 'direct cn', '["geosite:cn"]', 'direct', NULL, 1700000000, 1700000000);

-- prices are currency units: 9.99 → 999 cents, "99.9" → 9990, onetime 0 → not for sale
INSERT INTO "v2_plan" ("id", "group_id", "transfer_enable", "name", "prices", "sell", "speed_limit", "device_limit", "show", "sort", "renew", "content", "tags", "reset_traffic_method", "capacity_limit", "created_at", "updated_at") VALUES
(1, 1, 200, 'Monthly', '{"monthly":9.99,"yearly":"99.9","onetime":0}', 1, 0, 3, 1, 1, 1, 'Monthly plan', '["hot","new"]', NULL, NULL, 1700000000, 1700000000),
(2, NULL, NULL, 'Legacy', '{"quarterly":"","half_yearly":null,"weird":10}', 0, NULL, NULL, 0, NULL, 0, NULL, NULL, 4, 0, 1700000000, 1700000000);

-- expired_at 1801360800 = 2027-01-31 10:00 +08:00
INSERT INTO "v2_user" ("id", "invite_user_id", "telegram_id", "email", "password", "password_algo", "password_salt", "balance", "discount", "commission_type", "commission_rate", "commission_balance", "t", "u", "d", "transfer_enable", "banned", "is_admin", "last_login_at", "is_staff", "last_login_ip", "uuid", "group_id", "plan_id", "speed_limit", "remind_expire", "remind_traffic", "token", "expired_at", "remarks", "created_at", "updated_at", "device_limit", "online_count", "last_online_at", "next_reset_at", "last_reset_at", "reset_count") VALUES
(1, NULL, NULL, 'admin@example.com', '$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK', NULL, NULL, 0, NULL, 0, NULL, 0, 0, 0, 0, 0, 0, 1, 1700000000, 0, NULL, '10000000-0000-4000-8000-000000000001', 1, NULL, NULL, 1, 1, 'a0000000000000000000000000000001', NULL, NULL, 1690000000, 1700000000, NULL, 0, NULL, NULL, NULL, 0),
(2, 1, NULL, 'salted@example.com', 'd65cf2f6dad004d515d5ef2d41f7d99bf493e8dece1b17aacbc6c29a0e2fd00b', 'sha256salt', 'NaCl', 0, NULL, 0, 12.6, 0, 0, 100, 200, 214748364800, 0, 0, NULL, 0, NULL, '10000000-0000-4000-8000-000000000002', 1, 1, NULL, 1, 1, 'a0000000000000000000000000000002', 1801360800, NULL, 1690000000, 1700000000, 2, 1, '2026-10-20 10:00:00', 1790000000, NULL, 3);

INSERT INTO "v2_server_machine" ("id", "name", "token", "is_active", "created_at", "updated_at") VALUES
(1, 'machine-1', 'machine-example-token', 1, '2026-04-11 10:00:00', '2026-04-11 10:00:00');

-- created_at '2023-11-15 06:13:20' is unix 1700000000 in Asia/Shanghai
INSERT INTO "v2_server" ("id", "type", "code", "parent_id", "machine_id", "group_ids", "route_ids", "name", "rate", "rate_time_enable", "rate_time_ranges", "transfer_enable", "u", "d", "tags", "host", "port", "server_port", "protocol_settings", "custom_outbounds", "custom_routes", "cert_config", "show", "enabled", "sort", "created_at", "updated_at") VALUES
(1, 'shadowsocks', '1', NULL, NULL, '["1"]', '["1","2"]', 'SS-256', '1.00', 0, NULL, NULL, 0, 0, '["SS"]', 'ss.example.com', '443', 443, '{"cipher":"2022-blake3-aes-256-gcm","obfs":null,"obfs_settings":null,"plugin":null,"plugin_opts":null}', NULL, NULL, NULL, 1, 1, 1, '2023-11-15 06:13:20', '2023-11-15 06:13:20'),
(2, 'vmess', '5', NULL, 1, '["1","2"]', NULL, 'VM', '1.50', 0, NULL, NULL, 0, 0, NULL, 'vm.example.com', '443', 10086, '{"tls":1,"network":"ws","rules":null,"network_settings":{"path":"/vm"},"tls_settings":{"server_name":"vm.example.com","allow_insecure":false},"multiplex":{"enabled":true,"protocol":"yamux"},"utls":{"enabled":false,"fingerprint":"chrome"}}', NULL, '[{"domain":["example.com"],"outbound":"direct"}]', NULL, 1, 1, 2, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(3, 'vless', '3', NULL, NULL, '["2"]', NULL, 'VL-reality', '1.00', 0, NULL, NULL, 0, 0, NULL, 'vl.example.com', '443', 443, '{"tls":2,"tls_settings":null,"flow":"xtls-rprx-vision","network":"tcp","network_settings":null,"reality_settings":{"server_name":"www.example.com","server_port":"443","public_key":"example-public-key","private_key":"example-private-key","short_id":"ab","allow_insecure":false},"utls":{"enabled":true,"fingerprint":"firefox"},"encryption":{"enabled":false,"encryption":null,"decryption":null}}', NULL, NULL, NULL, 1, 1, 3, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(4, 'trojan', '4', NULL, NULL, '["1"]', NULL, 'TJ-disabled', '1', 1, '[{"start":"00:00","end":"08:00","rate":0.5}]', 1073741824, 0, 0, NULL, 'tj.example.com', '443', 443, '{"tls":1,"network":"grpc","network_settings":{"serviceName":"tj"},"server_name":"old.example.com","allow_insecure":false,"tls_settings":{"server_name":"tj.example.com","allow_insecure":true}}', NULL, NULL, NULL, 1, 0, 4, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(5, 'hysteria2', '5', NULL, NULL, '["1"]', NULL, 'HY2', '1', 0, NULL, NULL, 0, 0, NULL, 'hy.example.com', '30000-40000', 443, '{"version":2,"bandwidth":{"up":100,"down":500},"obfs":{"open":true,"type":"salamander","password":"parent-obfs-pass"},"tls":{"server_name":"hy.example.com","allow_insecure":false},"hop_interval":45}', NULL, NULL, NULL, 1, 1, 5, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(6, 'hysteria', '6', 5, NULL, '["1"]', NULL, 'HY2-relay', '1', 0, NULL, NULL, 0, 0, NULL, 'hy-relay.example.com', '443', 443, '{"version":2,"bandwidth":{"up":100,"down":500},"obfs":{"open":true,"type":"salamander","password":"child-obfs-pass"},"tls":{"server_name":"hy.example.com","allow_insecure":false}}', NULL, NULL, NULL, 1, NULL, 6, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(7, 'tuic', '7', NULL, NULL, '["1"]', NULL, 'TUIC', '1', 0, NULL, NULL, 0, 0, NULL, 'tuic.example.com', '443', 443, '{"version":5,"congestion_control":"bbr","alpn":["h3","h2"],"udp_relay_mode":"native","tls":{"server_name":"tuic.example.com","allow_insecure":false}}', NULL, NULL, NULL, 1, 1, 7, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(8, 'anytls', '8', NULL, NULL, '["1"]', NULL, 'AnyTLS', '1', 0, NULL, NULL, 0, 0, NULL, 'any.example.com', '443', 443, '{"padding_scheme":["stop=8","0=30-30"],"tls":{"server_name":"any.example.com","allow_insecure":true}}', NULL, NULL, NULL, 1, 1, 8, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(9, 'mieru', '9', NULL, NULL, '["1"]', NULL, 'Mieru', '1', 0, NULL, NULL, 0, 0, NULL, 'mieru.example.com', '443', 443, '{"transport":"UDP","traffic_pattern":"example-pattern","multiplex":{"enabled":false}}', NULL, NULL, NULL, 1, 1, 9, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(10, 'socks', '10', NULL, NULL, '["1"]', NULL, 'SOCKS', '1', 0, NULL, NULL, 0, 0, NULL, 'socks.example.com', '1080', 1080, '{"tls":0}', NULL, NULL, NULL, 1, 1, 10, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(11, 'v2ray', NULL, NULL, NULL, NULL, NULL, 'Old-v2ray', '1', 0, NULL, NULL, 0, 0, NULL, 'v2.example.com', '443', 443, '{"tls":0,"network":"tcp"}', NULL, NULL, NULL, 0, 1, NULL, '2024-01-01 08:00:00', '2024-01-01 08:00:00'),
(12, 'shadowsocks', '12', 1, NULL, '["1"]', NULL, 'SS-256-relay', '1', 0, NULL, NULL, 0, 0, NULL, 'ss-relay.example.com', '8443', 443, '{"cipher":"2022-blake3-aes-256-gcm"}', NULL, NULL, NULL, 1, 1, 12, '2024-06-01 08:00:00', '2024-06-01 08:00:00'),
(13, 'vmess', '13', NULL, NULL, '["1"]', NULL, 'Broken-JSON', '1', 0, NULL, NULL, 0, 0, NULL, 'broken.example.com', '443', 443, '{"tls":1,"network":', NULL, NULL, NULL, 1, 1, 13, '2024-01-01 08:00:00', '2024-01-01 08:00:00');

INSERT INTO "v2_plugins" ("id", "name", "code", "type", "version", "is_enabled", "config", "installed_at", "created_at", "updated_at") VALUES
(1, 'EPay', 'epay', 'payment', '1.0.0', 0, NULL, '2025-01-18 10:00:00', '2025-01-18 10:00:00', '2025-01-18 10:00:00'),
(2, 'BTCPay', 'btcpay', 'payment', '1.0.0', 1, NULL, '2025-01-18 10:00:00', '2025-01-18 10:00:00', '2025-01-18 10:00:00');

INSERT INTO "v2_payment" ("id", "uuid", "payment", "name", "icon", "config", "notify_domain", "handling_fee_fixed", "handling_fee_percent", "enable", "sort", "created_at", "updated_at") VALUES
(1, 'UpEp1234', 'EPay', 'EPay', NULL, '{"url":"https://pay.example.com","pid":"2002","key":"epay-example-key","type":"usdt.tron"}', NULL, NULL, NULL, 1, 1, 1700000000, 1700000000),
(2, 'UpBt5678', 'BTCPay', 'BTCPay', NULL, '{"btcpay_url":"https://btcpay.example.com","btcpay_storeId":"store2","btcpay_api_key":"btc-example-key","btcpay_webhook_key":"btc-example-hook"}', NULL, NULL, NULL, 1, 2, 1700000000, 1700000000);

-- ticket 1: no messages, fixed semantics (reply_status 1 = replied); ticket 2: the user replied last
INSERT INTO "v2_ticket" ("id", "user_id", "subject", "level", "status", "reply_status", "created_at", "updated_at", "last_reply_user_id") VALUES
(1, 2, 'Fixed semantics', 0, 0, 1, 1700000000, 1700000000, 1),
(2, 2, 'User replied last', 1, 0, 1, 1700000000, 1700000000, 1);

INSERT INTO "v2_ticket_message" ("id", "user_id", "ticket_id", "message", "created_at", "updated_at") VALUES
(1, 1, 2, 'Hello', 1700000000, 1700000000),
(2, 2, 2, 'Still broken', 1700000100, 1700000100);

INSERT INTO "v2_notice" ("id", "title", "content", "show", "img_url", "tags", "created_at", "updated_at", "sort") VALUES
(1, 'Sorted', 'content', 1, NULL, '["a"]', 1700000000, 1700000000, 5),
(2, 'Unsorted', 'content', 1, NULL, NULL, 1700000000, 1700000000, NULL);

INSERT INTO "v2_knowledge" ("id", "language", "category", "title", "body", "sort", "show", "created_at", "updated_at") VALUES
(1, 'en-US', 'Guide', 'macOS', 'Install the client', 2, 1, 1700000000, 1700000000);

INSERT INTO "v2_coupon" ("id", "code", "name", "type", "value", "show", "limit_use", "limit_use_with_user", "limit_plan_ids", "limit_period", "started_at", "ended_at", "created_at", "updated_at") VALUES
(1, 'UPSTREAM', 'Upstream', 2, 15, 1, NULL, NULL, '[1]', '["monthly","yearly"]', 1700000000, 1900000000, 1700000000, 1700000000);

INSERT INTO "v2_invite_code" ("id", "user_id", "code", "status", "pv", "created_at", "updated_at") VALUES
(1, 1, 'UPINVITE', 0, 0, 1700000000, 1700000000);

INSERT INTO "v2_commission_log" ("id", "invite_user_id", "user_id", "trade_no", "order_amount", "get_amount", "created_at", "updated_at") VALUES
(1, 1, 2, '2025010112000012345678', 999, 99, 1700000000, 1700000000);

-- settings: U booleans, turnstile captcha, an empty server_token, no reset_traffic_method
INSERT INTO "v2_settings" ("id", "group", "type", "name", "value", "created_at", "updated_at") VALUES
(1, NULL, NULL, 'app_name', 'Upstream Board', '2025-06-21 10:00:00', '2025-06-21 10:00:00'),
(2, NULL, NULL, 'server_token', '', NULL, NULL),
(3, NULL, NULL, 'captcha_enable', '1', NULL, NULL),
(4, NULL, NULL, 'captcha_type', 'turnstile', NULL, NULL),
(5, NULL, NULL, 'turnstile_site_key', 'turnstile-example-site', NULL, NULL),
(6, NULL, NULL, 'turnstile_secret_key', 'turnstile-example-secret', NULL, NULL),
(7, NULL, NULL, 'subscribe_path', 'sub', NULL, NULL),
(8, NULL, NULL, 'email_encryption', 'tls', NULL, NULL),
(9, NULL, NULL, 'device_limit_mode', '1', NULL, NULL),
(10, NULL, NULL, 'server_pull_interval', '5', NULL, NULL),
(11, NULL, NULL, 'stop_register', 'false', NULL, NULL),
(12, NULL, NULL, 'email_whitelist_suffix', '["example.com","example.org"]', NULL, NULL),
(13, NULL, NULL, 'commission_withdraw_method', '["USDT"]', NULL, NULL),
(14, NULL, NULL, 'currency', 'USD', NULL, NULL);

INSERT INTO "v2_stat_server" ("id", "server_id", "server_type", "u", "d", "record_type", "record_at", "created_at", "updated_at") VALUES
(1, 5, 'hysteria', 50, 60, 'd', 1729440000, 1729440000, 1729440000),
(2, 5, 'vmess', 10, 10, 'd', 1729440000, 1729440000, 1729440000);

INSERT INTO "v2_stat_user" ("id", "user_id", "server_rate", "u", "d", "record_type", "record_at", "created_at", "updated_at") VALUES
(1, 2, 2.00, 200, 400, 'd', 1729440000, 1729440000, 1729440000);

INSERT INTO "v2_gift_card_template" ("id", "name", "type", "rewards", "admin_id", "created_at", "updated_at") VALUES
(1, 'Example card', 1, '{"balance":100}', 1, 1700000000, 1700000000);
