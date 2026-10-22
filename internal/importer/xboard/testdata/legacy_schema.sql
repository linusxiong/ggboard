-- Xboard legacy schema (linusxiong/Xboard fork, Laravel 10): the result of its 33 migrations in
-- database/migrations, transcribed to the SQLite dialect Laravel's schema builder produces.
-- Booleans are INTEGER, bigInteger() columns BIGINT. Columns added by later migrations are appended at the end (SQLite ignores ->after()).
-- As in the migrations, v2_stat and v2_stat_user have no unique indexes on SQLite, and v2_settings.name has no
-- unique index (the migration calls ->uniqid(), a typo for ->unique()).

CREATE TABLE "migrations" ("id" integer primary key autoincrement not null, "migration" varchar not null, "batch" integer not null);

-- 2019_08_19_000000_create_failed_jobs_table
CREATE TABLE "failed_jobs" ("id" integer primary key autoincrement not null, "connection" text not null, "queue" text not null, "payload" text not null, "exception" text not null, "failed_at" datetime not null default CURRENT_TIMESTAMP);

-- 2023_08_07_205816_create_v2_commission_log_table
CREATE TABLE "v2_commission_log" ("id" integer primary key autoincrement not null, "invite_user_id" integer not null, "user_id" integer not null, "trade_no" varchar(36) not null, "order_amount" integer not null, "get_amount" integer not null, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_coupon_table
CREATE TABLE "v2_coupon" ("id" integer primary key autoincrement not null, "code" varchar not null, "name" varchar not null, "type" INTEGER not null, "value" integer not null, "show" INTEGER not null default '0', "limit_use" integer, "limit_use_with_user" integer, "limit_plan_ids" varchar, "limit_period" varchar, "started_at" integer not null, "ended_at" integer not null, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_invite_code_table
CREATE TABLE "v2_invite_code" ("id" integer primary key autoincrement not null, "user_id" integer not null, "code" varchar(32) not null, "status" INTEGER not null default '0', "pv" integer not null default '0', "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_knowledge_table
CREATE TABLE "v2_knowledge" ("id" integer primary key autoincrement not null, "language" varchar(5) not null, "category" varchar not null, "title" varchar not null, "body" text not null, "sort" integer, "show" INTEGER not null default '0', "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_log_table
CREATE TABLE "v2_log" ("id" integer primary key autoincrement not null, "title" text not null, "level" varchar(11), "host" varchar, "uri" varchar not null, "method" varchar(11) not null, "data" text, "ip" varchar(128), "context" text, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_mail_log_table
CREATE TABLE "v2_mail_log" ("id" integer primary key autoincrement not null, "email" varchar(64) not null, "subject" varchar not null, "template_name" varchar not null, "error" text, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_notice_table
CREATE TABLE "v2_notice" ("id" integer primary key autoincrement not null, "title" varchar not null, "content" text not null, "show" INTEGER not null default '0', "img_url" varchar, "tags" varchar, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_order_table
CREATE TABLE "v2_order" ("id" integer primary key autoincrement not null, "invite_user_id" integer, "user_id" integer not null, "plan_id" integer not null, "coupon_id" integer, "payment_id" integer, "type" integer not null, "period" varchar not null, "trade_no" varchar(36) not null, "callback_no" varchar, "total_amount" integer not null, "handling_amount" integer, "discount_amount" integer, "surplus_amount" integer, "refund_amount" integer, "balance_amount" integer, "surplus_order_ids" text, "status" INTEGER not null default '0', "commission_status" INTEGER not null default '0', "commission_balance" integer not null default '0', "actual_commission_balance" integer, "paid_at" integer, "created_at" integer not null, "updated_at" integer not null);
CREATE UNIQUE INDEX "trade_no" on "v2_order" ("trade_no");

-- 2023_08_07_205816_create_v2_payment_table (+ 2024_03_19_103149_modify_icon_column_to_v2_payment_table: icon text)
CREATE TABLE "v2_payment" ("id" integer primary key autoincrement not null, "uuid" varchar(32) not null, "payment" varchar(16) not null, "name" varchar not null, "icon" text, "config" text not null, "notify_domain" varchar(128), "handling_fee_fixed" integer, "handling_fee_percent" numeric, "enable" INTEGER not null default '0', "sort" integer, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_plan_table (reset_traffic_method is a boolean() column holding 0..4)
CREATE TABLE "v2_plan" ("id" integer primary key autoincrement not null, "group_id" integer not null, "transfer_enable" integer not null, "name" varchar not null, "speed_limit" integer, "show" INTEGER not null default '0', "sort" integer, "renew" INTEGER not null default '1', "content" text, "month_price" integer, "quarter_price" integer, "half_year_price" integer, "year_price" integer, "two_year_price" integer, "three_year_price" integer, "onetime_price" integer, "reset_price" integer, "reset_traffic_method" INTEGER, "capacity_limit" integer, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_server_group_table
CREATE TABLE "v2_server_group" ("id" integer primary key autoincrement not null, "name" varchar not null, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_server_hysteria_table
-- + 2023_09_04 excludes, 2023_09_06 ips, 2023_09_14 alpn, 2023_09_29 version / is_obfs
CREATE TABLE "v2_server_hysteria" ("id" integer primary key autoincrement not null, "group_id" varchar not null, "route_id" varchar, "name" varchar not null, "parent_id" integer, "host" varchar not null, "port" varchar(11) not null, "server_port" integer not null, "tags" varchar, "rate" varchar(11) not null, "show" INTEGER not null default '0', "sort" integer, "up_mbps" integer not null, "down_mbps" integer not null, "server_name" varchar(64), "insecure" INTEGER not null default '0', "created_at" integer not null, "updated_at" integer not null, "excludes" text, "ips" varchar, "alpn" integer not null default '0', "version" integer not null default '1', "is_obfs" INTEGER not null default '1');

-- 2023_08_07_205816_create_v2_server_route_table
CREATE TABLE "v2_server_route" ("id" integer primary key autoincrement not null, "remarks" varchar not null, "match" text not null, "action" varchar(11) not null, "action_value" varchar, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_server_shadowsocks_table (+ excludes, ips)
CREATE TABLE "v2_server_shadowsocks" ("id" integer primary key autoincrement not null, "group_id" varchar not null, "route_id" varchar, "parent_id" integer, "tags" varchar, "name" varchar not null, "rate" varchar(11) not null, "host" varchar not null, "port" varchar(11) not null, "server_port" integer not null, "cipher" varchar not null, "obfs" varchar(11), "obfs_settings" varchar, "show" integer not null default '0', "sort" integer, "created_at" integer not null, "updated_at" integer not null, "excludes" text, "ips" varchar);

-- 2023_08_07_205816_create_v2_server_trojan_table (+ excludes, ips, 2023_09_24 network / networkSettings)
CREATE TABLE "v2_server_trojan" ("id" integer primary key autoincrement not null, "group_id" varchar not null, "route_id" varchar, "parent_id" integer, "tags" varchar, "name" varchar not null, "rate" varchar(11) not null, "host" varchar not null, "port" varchar(11) not null, "server_port" integer not null, "allow_insecure" INTEGER not null default '0', "server_name" varchar, "show" INTEGER not null default '0', "sort" integer, "created_at" integer not null, "updated_at" integer not null, "excludes" text, "ips" varchar, "network" varchar(11) not null default 'tcp', "networkSettings" text);

-- 2023_08_07_205816_create_v2_server_vless_table (+ excludes, ips; tls is a boolean() column holding 0..2)
CREATE TABLE "v2_server_vless" ("id" integer primary key autoincrement not null, "group_id" text not null, "route_id" text, "name" varchar not null, "parent_id" integer, "host" varchar not null, "port" integer not null, "server_port" integer not null, "tls" INTEGER not null, "tls_settings" text, "flow" varchar(64), "network" varchar(11) not null, "network_settings" text, "tags" text, "rate" varchar(11) not null, "show" INTEGER not null default '0', "sort" integer, "created_at" integer not null, "updated_at" integer not null, "excludes" text, "ips" varchar);

-- 2023_08_07_205816_create_v2_server_vmess_table (+ excludes, ips)
CREATE TABLE "v2_server_vmess" ("id" integer primary key autoincrement not null, "group_id" varchar not null, "route_id" varchar, "name" varchar not null, "parent_id" integer, "host" varchar not null, "port" varchar(11) not null, "server_port" integer not null, "tls" integer not null default '0', "tags" varchar, "rate" varchar(11) not null, "network" varchar(11) not null, "rules" text, "networkSettings" text, "tlsSettings" text, "ruleSettings" text, "dnsSettings" text, "show" INTEGER not null default '0', "sort" integer, "created_at" integer not null, "updated_at" integer not null, "excludes" text, "ips" varchar);

-- 2023_08_07_205816_create_v2_stat_server_table
CREATE TABLE "v2_stat_server" ("id" integer primary key autoincrement not null, "server_id" integer not null, "server_type" varchar(11) not null, "u" bigint not null, "d" bigint not null, "record_type" varchar(1) not null, "record_at" integer not null, "created_at" integer not null, "updated_at" integer not null);
CREATE INDEX "server_id" on "v2_stat_server" ("server_id");
CREATE INDEX "record_at" on "v2_stat_server" ("record_at");
CREATE UNIQUE INDEX "server_id_server_type_record_at" on "v2_stat_server" ("server_id", "server_type", "record_at");

-- 2023_08_07_205816_create_v2_stat_table (no unique record_at on sqlite)
CREATE TABLE "v2_stat" ("id" integer primary key autoincrement not null, "record_at" integer not null, "record_type" varchar(1) not null, "order_count" integer not null, "order_total" integer not null, "commission_count" integer not null, "commission_total" integer not null, "paid_count" integer not null, "paid_total" integer not null, "register_count" integer not null, "invite_count" integer not null, "transfer_used_total" varchar(32) not null, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_stat_user_table (no indexes on sqlite)
CREATE TABLE "v2_stat_user" ("id" integer primary key autoincrement not null, "user_id" integer not null, "server_rate" numeric not null, "u" bigint not null, "d" bigint not null, "record_type" varchar(2) not null, "record_at" integer not null, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_ticket_message_table
CREATE TABLE "v2_ticket_message" ("id" integer primary key autoincrement not null, "user_id" integer not null, "ticket_id" integer not null, "message" text not null, "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_ticket_table
CREATE TABLE "v2_ticket" ("id" integer primary key autoincrement not null, "user_id" integer not null, "subject" varchar not null, "level" INTEGER not null, "status" INTEGER not null default '0', "reply_status" INTEGER not null default '1', "created_at" integer not null, "updated_at" integer not null);

-- 2023_08_07_205816_create_v2_user_table (+ 2023_12_12_212239_add_index_to_v2_user_table)
CREATE TABLE "v2_user" ("id" integer primary key autoincrement not null, "invite_user_id" integer, "telegram_id" bigint, "email" varchar(64) not null, "password" varchar(64) not null, "password_algo" varchar(10), "password_salt" varchar(10), "balance" integer not null default '0', "discount" integer, "commission_type" integer not null default '0', "commission_rate" integer, "commission_balance" integer not null default '0', "t" integer not null default '0', "u" bigint not null default '0', "d" bigint not null default '0', "transfer_enable" bigint not null default '0', "banned" INTEGER not null default '0', "is_admin" INTEGER not null default '0', "last_login_at" integer, "is_staff" INTEGER not null default '0', "last_login_ip" integer, "uuid" varchar(36) not null, "group_id" integer, "plan_id" integer, "speed_limit" integer, "remind_expire" integer default '1', "remind_traffic" integer default '1', "token" varchar(32) not null, "expired_at" bigint default '0', "remarks" text, "created_at" integer not null, "updated_at" integer not null);
CREATE UNIQUE INDEX "email" on "v2_user" ("email");
CREATE INDEX "v2_user_u_d_expired_at_group_id_banned_transfer_enable_index" on "v2_user" ("u", "d", "expired_at", "group_id", "banned", "transfer_enable");

-- 2023_08_14_221234_create_v2_settings_table (+ 2023_11_19_205026 value → text)
CREATE TABLE "v2_settings" ("id" integer primary key autoincrement not null, "group" varchar, "type" varchar, "name" varchar not null, "value" text, "created_at" datetime, "updated_at" datetime);
