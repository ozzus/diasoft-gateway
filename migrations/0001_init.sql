create table if not exists verification_records (
    diploma_id uuid primary key,
    verification_token varchar(255) not null unique,
    university_code varchar(64) not null,
    diploma_number varchar(128) not null,
    student_name_masked varchar(255) not null,
    program_name varchar(255) not null,
    status varchar(32) not null,
    updated_at timestamptz not null default now()
);

create table if not exists share_link_records (
    share_token varchar(255) primary key,
    diploma_id uuid not null,
    expires_at timestamptz not null,
    max_views integer,
    used_views integer not null default 0,
    status varchar(32) not null,
    updated_at timestamptz not null default now()
);

create table if not exists verification_audit (
    id uuid primary key,
    request_type varchar(64) not null,
    token varchar(255),
    diploma_number varchar(128),
    university_code varchar(64),
    remote_ip varchar(128),
    verdict varchar(32) not null,
    created_at timestamptz not null default now()
);

create table if not exists processed_events (
    event_id varchar(255) primary key,
    event_type varchar(128) not null,
    processed_at timestamptz not null default now()
);
