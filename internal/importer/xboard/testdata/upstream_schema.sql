-- Xboard upstream schema (cedar2025/Xboard master, 2026-05): the result of running every migration in
-- database/migrations, transcribed to the SQLite dialect Laravel's schema builder produces. Booleans are
-- INTEGER, bigInteger() BIGINT, json columns TEXT, timestamp()/timestamps() DATETIME. Tables rebuilt by ->change() list their
-- columns in the final order; added columns are appended.
-- Notable migrations: 2025_01_04 optimize_plan (prices JSON, sell; price columns dropped), 2025_01_05
-- create_v2_server (unified table; per-protocol tables dropped), 2025_01_10 device_limit, 2025_01_12 notice sort,
-- 2025_06_21 next_reset_at / traffic reset logs, 2025_07_01 plan tags / gift cards, 2025_07_13 rate_time_ranges,
-- 2025_07_27 subscribe templates, 2026_03_* audit log / custom config / node traffic, 2026_04_11 machines + enabled,
-- 2026_04_20 mail templates, 2026_04_21 last_reply_user_id, 2026_05_30 refund_amount → surplus_credit.

CREATE TABLE "migrations" ("id" integer primary key autoincrement not null, "migration" varchar not null, "batch" integer not null);

CREATE TABLE "failed_jobs" ("id" integer primary key autoincrement not null, "connection" text not null, "queue" text not null, "payload" text not null, "exception" text not null, "failed_at" datetime not null default CURRENT_TIMESTAMP);

CREATE TABLE "personal_access_tokens" ("id" integer primary key autoincrement not null, "tokenable_type" varchar not null, "tokenable_id" bigint not null, "name" varchar not null, "token" varchar not null, "abilities" text, "last_used_at" datetime, "expires_at" datetime, "created_at" datetime, "updated_at" datetime);
CREATE INDEX "personal_access_tokens_tokenable_type_tokenable_id_index" on "personal_access_tokens" ("tokenable_type", "tokenable_id");
CREATE UNIQUE INDEX "personal_access_tokens_token_unique" on "personal_access_tokens" ("token");

CREATE TABLE "v2_commission_log" ("id" integer primary key autoincrement not null, "invite_user_id" integer not null, "user_id" integer not null, "trade_no" varchar(36) not null, "order_amount" integer not null, "get_amount" integer not null, "created_at" integer not null, "updated_at" integer not null);
CREATE INDEX "v2_commission_log_created_at_index" on "v2_commission_log" ("created_at");
CREATE INDEX "v2_commission_log_get_amount_index" on "v2_commission_log" ("get_amount");

CREATE TABLE "v2_invite_code" ("id" integer primary key autoincrement not null, "user_id" integer not null, "code" varchar(32) not null, "status" INTEGER not null default '0', "pv" integer not null default '0', "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_knowledge" ("id" integer primary key autoincrement not null, "language" varchar(5) not null, "category" varchar not null, "title" varchar not null, "body" text not null, "sort" integer, "show" INTEGER not null default '0', "created_at" integer not null, "updated_at" integer not null);

-- 2025_01_04_optimize_plan_table, 2025_01_10 device_limit, 2025_07_01 tags
CREATE TABLE "v2_plan" ("id" integer primary key autoincrement not null, "group_id" integer, "transfer_enable" bigint, "name" varchar not null, "prices" text, "sell" INTEGER not null default '0', "speed_limit" integer, "device_limit" integer, "show" INTEGER not null default '0', "sort" integer, "renew" INTEGER not null default '1', "content" text, "tags" text, "reset_traffic_method" integer default '0', "capacity_limit" integer default '0', "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_server_group" ("id" integer primary key autoincrement not null, "name" varchar not null, "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_server_route" ("id" integer primary key autoincrement not null, "remarks" varchar not null, "match" text not null, "action" varchar(11) not null, "action_value" varchar, "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_stat_server" ("id" integer primary key autoincrement not null, "server_id" integer not null, "server_type" varchar(11) not null, "u" bigint not null, "d" bigint not null, "record_type" varchar(1) not null, "record_at" integer not null, "created_at" integer not null, "updated_at" integer not null);
CREATE INDEX "server_id" on "v2_stat_server" ("server_id");
CREATE INDEX "record_at" on "v2_stat_server" ("record_at");
CREATE UNIQUE INDEX "server_id_server_type_record_at" on "v2_stat_server" ("server_id", "server_type", "record_at");
CREATE INDEX "v2_stat_server_u_index" on "v2_stat_server" ("u");
CREATE INDEX "v2_stat_server_d_index" on "v2_stat_server" ("d");

-- 2025_01_10 device_limit / online_count / last_online_at, 2025_06_21 next_reset_at / last_reset_at / reset_count
CREATE TABLE "v2_user" ("id" integer primary key autoincrement not null, "invite_user_id" integer, "telegram_id" bigint, "email" varchar(64) not null, "password" varchar(64) not null, "password_algo" varchar(10), "password_salt" varchar(10), "balance" integer not null default '0', "discount" integer, "commission_type" integer not null default '0', "commission_rate" integer, "commission_balance" integer not null default '0', "t" integer not null default '0', "u" bigint not null default '0', "d" bigint not null default '0', "transfer_enable" bigint not null default '0', "banned" INTEGER not null default '0', "is_admin" INTEGER not null default '0', "last_login_at" integer, "is_staff" INTEGER not null default '0', "last_login_ip" integer, "uuid" varchar(36) not null, "group_id" integer, "plan_id" integer, "speed_limit" integer, "remind_expire" integer default '1', "remind_traffic" integer default '1', "token" varchar(32) not null, "expired_at" bigint default '0', "remarks" text, "created_at" integer not null, "updated_at" integer not null, "device_limit" integer, "online_count" integer, "last_online_at" datetime, "next_reset_at" integer, "last_reset_at" integer, "reset_count" integer not null default '0');
CREATE UNIQUE INDEX "email" on "v2_user" ("email");
CREATE INDEX "v2_user_u_d_expired_at_group_id_banned_transfer_enable_index" on "v2_user" ("u", "d", "expired_at", "group_id", "banned", "transfer_enable");
CREATE INDEX "v2_user_t_index" on "v2_user" ("t");
CREATE INDEX "v2_user_online_count_index" on "v2_user" ("online_count");
CREATE INDEX "v2_user_created_at_index" on "v2_user" ("created_at");
CREATE INDEX "idx_next_reset_at" on "v2_user" ("next_reset_at");

CREATE TABLE "v2_mail_log" ("id" integer primary key autoincrement not null, "email" varchar(64) not null, "subject" varchar not null, "template_name" varchar not null, "error" text, "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_log" ("id" integer primary key autoincrement not null, "title" text not null, "level" varchar(11), "host" varchar, "uri" varchar not null, "method" varchar(11) not null, "data" text, "ip" varchar(128), "context" text, "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_stat" ("id" integer primary key autoincrement not null, "record_at" integer not null, "record_type" varchar(1) not null, "order_count" integer not null, "order_total" integer not null, "commission_count" integer not null, "commission_total" integer not null, "paid_count" integer not null, "paid_total" integer not null, "register_count" integer not null, "invite_count" integer not null, "transfer_used_total" varchar(32) not null, "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_stat_user" ("id" integer primary key autoincrement not null, "user_id" integer not null, "server_rate" numeric not null, "u" bigint not null, "d" bigint not null, "record_type" varchar(2) not null, "record_at" integer not null, "created_at" integer not null, "updated_at" integer not null);
CREATE INDEX "v2_stat_user_u_index" on "v2_stat_user" ("u");
CREATE INDEX "v2_stat_user_d_index" on "v2_stat_user" ("d");
CREATE INDEX "idx_stat_user_record_user" on "v2_stat_user" ("record_at", "user_id");

CREATE TABLE "v2_ticket_message" ("id" integer primary key autoincrement not null, "user_id" integer not null, "ticket_id" integer not null, "message" text not null, "created_at" integer not null, "updated_at" integer not null);

-- 2025_01_01 / 2025_01_12 commission_status nullable, 2026_05_30 refund_amount → surplus_credit
CREATE TABLE "v2_order" ("id" integer primary key autoincrement not null, "invite_user_id" integer, "user_id" integer not null, "plan_id" integer not null, "coupon_id" integer, "payment_id" integer, "type" integer not null, "period" varchar not null, "trade_no" varchar(36) not null, "callback_no" varchar, "total_amount" integer not null, "handling_amount" integer, "discount_amount" integer, "surplus_amount" integer, "surplus_credit" integer, "balance_amount" integer, "surplus_order_ids" text, "status" integer not null default '0', "commission_status" integer, "commission_balance" integer not null default '0', "actual_commission_balance" integer, "paid_at" integer, "created_at" integer not null, "updated_at" integer not null);
CREATE UNIQUE INDEX "trade_no" on "v2_order" ("trade_no");
CREATE INDEX "v2_order_created_at_index" on "v2_order" ("created_at");
CREATE INDEX "v2_order_status_index" on "v2_order" ("status");
CREATE INDEX "v2_order_updated_at_index" on "v2_order" ("updated_at");

CREATE TABLE "v2_payment" ("id" integer primary key autoincrement not null, "uuid" varchar(32) not null, "payment" varchar(16) not null, "name" varchar not null, "icon" text, "config" text not null, "notify_domain" varchar(128), "handling_fee_fixed" integer, "handling_fee_percent" numeric, "enable" INTEGER not null default '0', "sort" integer, "created_at" integer not null, "updated_at" integer not null);

CREATE TABLE "v2_coupon" ("id" integer primary key autoincrement not null, "code" varchar not null, "name" varchar not null, "type" integer not null, "value" integer not null, "show" INTEGER not null default '0', "limit_use" integer, "limit_use_with_user" integer, "limit_plan_ids" varchar, "limit_period" varchar, "started_at" integer not null, "ended_at" integer not null, "created_at" integer not null, "updated_at" integer not null);

-- 2025_01_12_190315_add_sort_to_v2_notice_table
CREATE TABLE "v2_notice" ("id" integer primary key autoincrement not null, "title" varchar not null, "content" text not null, "show" INTEGER not null default '0', "img_url" varchar, "tags" varchar, "created_at" integer not null, "updated_at" integer not null, "sort" integer);
CREATE INDEX "v2_notice_sort_index" on "v2_notice" ("sort");

-- 2026_04_21_000001_fix_ticket_reply_status (last_reply_user_id; reply_status values swapped, default 0)
CREATE TABLE "v2_ticket" ("id" integer primary key autoincrement not null, "user_id" integer not null, "subject" varchar not null, "level" integer not null, "status" integer not null default '0', "reply_status" integer not null default '0', "created_at" integer not null, "updated_at" integer not null, "last_reply_user_id" integer);
CREATE INDEX "v2_ticket_status_index" on "v2_ticket" ("status");
CREATE INDEX "v2_ticket_created_at_index" on "v2_ticket" ("created_at");

-- 2023_08_14 + 2025_06_21_000001_optimize_v2_settings_table
CREATE TABLE "v2_settings" ("id" integer primary key autoincrement not null, "group" varchar, "type" varchar, "name" varchar not null, "value" text, "created_at" datetime, "updated_at" datetime);
CREATE UNIQUE INDEX "v2_settings_name_unique" on "v2_settings" ("name");
CREATE INDEX "idx_setting_name" on "v2_settings" ("name");

-- 2026_04_11_000001_add_machine_support
CREATE TABLE "v2_server_machine" ("id" integer primary key autoincrement not null, "name" varchar not null, "token" varchar not null, "notes" text, "is_active" INTEGER not null default '1', "last_seen_at" integer, "load_status" text, "created_at" datetime, "updated_at" datetime);
CREATE UNIQUE INDEX "v2_server_machine_token_unique" on "v2_server_machine" ("token");
CREATE TABLE "v2_server_machine_load_history" ("id" integer primary key autoincrement not null, "machine_id" bigint not null, "cpu" float not null default '0', "mem_total" bigint not null default '0', "mem_used" bigint not null default '0', "disk_total" bigint not null default '0', "disk_used" bigint not null default '0', "net_in_speed" double, "net_out_speed" double, "recorded_at" integer not null, "created_at" datetime, "updated_at" datetime, foreign key("machine_id") references "v2_server_machine"("id") on delete cascade);
CREATE INDEX "v2_server_machine_load_history_machine_id_recorded_at_index" on "v2_server_machine_load_history" ("machine_id", "recorded_at");

-- 2025_01_05_131425_create_v2_server_table, 2025_07_13 rate_time_*, 2026_03_15 custom config / cert,
-- 2026_03_28 node traffic, 2026_04_11 machine_id / enabled, 2026_04_18 enabled nullable
CREATE TABLE "v2_server" ("id" integer primary key autoincrement not null, "type" varchar not null, "code" varchar, "parent_id" integer, "machine_id" bigint, "group_ids" text, "route_ids" text, "name" varchar not null, "rate" numeric not null, "rate_time_enable" INTEGER not null default '0', "rate_time_ranges" text, "transfer_enable" bigint, "u" bigint not null default '0', "d" bigint not null default '0', "tags" text, "host" varchar not null, "port" varchar not null, "server_port" integer not null, "protocol_settings" text, "custom_outbounds" text, "custom_routes" text, "cert_config" text, "show" INTEGER not null default '0', "enabled" INTEGER default '1', "sort" integer, "created_at" datetime, "updated_at" datetime, foreign key("machine_id") references "v2_server_machine"("id") on delete set null);
CREATE UNIQUE INDEX "v2_server_type_code_unique" on "v2_server" ("type", "code");
CREATE INDEX "v2_server_sort_index" on "v2_server" ("sort");

-- 2025_01_18_140511_create_plugins_table, 2025_07_26_000001_add_type_to_plugins_table
CREATE TABLE "v2_plugins" ("id" integer primary key autoincrement not null, "name" varchar not null, "code" varchar not null, "type" varchar(20) not null default 'feature', "version" varchar(50) not null, "is_enabled" INTEGER not null default '0', "config" text, "installed_at" datetime, "created_at" datetime, "updated_at" datetime);
CREATE UNIQUE INDEX "v2_plugins_code_unique" on "v2_plugins" ("code");
CREATE INDEX "v2_plugins_type_is_enabled_index" on "v2_plugins" ("type", "is_enabled");

-- 2025_06_21_000002_create_traffic_reset_logs_table
CREATE TABLE "v2_traffic_reset_logs" ("id" integer primary key autoincrement not null, "user_id" bigint not null, "reset_type" varchar(50) not null, "reset_time" datetime not null, "old_upload" bigint not null default '0', "old_download" bigint not null default '0', "old_total" bigint not null default '0', "new_upload" bigint not null default '0', "new_download" bigint not null default '0', "new_total" bigint not null default '0', "trigger_source" varchar(50) not null, "metadata" text, "created_at" datetime, "updated_at" datetime);
CREATE INDEX "idx_user_id" on "v2_traffic_reset_logs" ("user_id");
CREATE INDEX "idx_reset_time" on "v2_traffic_reset_logs" ("reset_time");
CREATE INDEX "idx_user_reset_time" on "v2_traffic_reset_logs" ("user_id", "reset_time");

-- 2025_07_01_122908_create_gift_card_tables
CREATE TABLE "v2_gift_card_template" ("id" integer primary key autoincrement not null, "name" varchar not null, "description" text, "type" integer not null, "status" integer not null default '1', "conditions" text, "rewards" text not null, "limits" text, "special_config" text, "icon" varchar, "background_image" varchar, "theme_color" varchar(7) not null default '#1890ff', "sort" integer not null default '0', "admin_id" integer not null, "created_at" integer not null, "updated_at" integer not null);
CREATE TABLE "v2_gift_card_code" ("id" integer primary key autoincrement not null, "template_id" integer not null, "code" varchar(32) not null, "batch_id" varchar(32), "status" integer not null default '0', "user_id" integer, "used_at" integer, "expires_at" integer, "actual_rewards" text, "usage_count" integer not null default '0', "max_usage" integer not null default '1', "metadata" text, "created_at" integer not null, "updated_at" integer not null);
CREATE UNIQUE INDEX "v2_gift_card_code_code_unique" on "v2_gift_card_code" ("code");
CREATE TABLE "v2_gift_card_usage" ("id" integer primary key autoincrement not null, "code_id" integer not null, "template_id" integer not null, "user_id" integer not null, "invite_user_id" integer, "rewards_given" text not null, "invite_rewards" text, "user_level_at_use" integer, "plan_id_at_use" integer, "multiplier_applied" numeric not null default '1', "ip_address" varchar(45), "user_agent" text, "notes" text, "created_at" integer not null);

-- 2025_07_27_000001_create_v2_subscribe_templates_table
CREATE TABLE "v2_subscribe_templates" ("id" integer primary key autoincrement not null, "name" varchar not null, "content" text, "created_at" datetime, "updated_at" datetime);
CREATE UNIQUE INDEX "v2_subscribe_templates_name_unique" on "v2_subscribe_templates" ("name");

-- 2026_03_11_000001_replace_v2_log_with_admin_audit_log
CREATE TABLE "v2_admin_audit_log" ("id" integer primary key autoincrement not null, "admin_id" bigint not null, "action" varchar(64) not null, "method" varchar(10) not null, "uri" varchar(512) not null, "request_data" text, "ip" varchar(128), "created_at" integer not null, "updated_at" integer not null);
CREATE INDEX "v2_admin_audit_log_admin_id_index" on "v2_admin_audit_log" ("admin_id");
CREATE INDEX "v2_admin_audit_log_action_index" on "v2_admin_audit_log" ("action");

-- 2026_04_20_000001_create_mail_templates_table
CREATE TABLE "v2_mail_templates" ("id" integer primary key autoincrement not null, "name" varchar(64) not null, "subject" varchar(255) not null, "content" text not null, "created_at" datetime, "updated_at" datetime);
CREATE UNIQUE INDEX "v2_mail_templates_name_unique" on "v2_mail_templates" ("name");
