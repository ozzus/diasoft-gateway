create table if not exists auth_users (
    id uuid primary key,
    login varchar(255) not null,
    password_hash varchar(255) not null,
    name varchar(255) not null,
    role varchar(32) not null,
    organization_code varchar(64),
    university_id uuid,
    diploma_id uuid,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (login, role)
);

create index if not exists idx_auth_users_role on auth_users(role);

insert into auth_users (id, login, password_hash, name, role, organization_code, university_id)
values (
    '11111111-aaaa-aaaa-aaaa-111111111111',
    'ITMO',
    '$2a$10$7f2jyY18zCn5LeyJt2jP9Of0WixOkOTahKsBgd98iWlu9M6QXtMAy',
    'ITMO Operator',
    'university',
    'ITMO',
    '11111111-1111-1111-1111-111111111111'
)
on conflict (login, role) do nothing;

insert into auth_users (id, login, password_hash, name, role, diploma_id)
values (
    '22222222-bbbb-bbbb-bbbb-222222222222',
    'D-2026-0001',
    '$2a$10$7f2jyY18zCn5LeyJt2jP9Of0WixOkOTahKsBgd98iWlu9M6QXtMAy',
    'Иван Иванов',
    'student',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
)
on conflict (login, role) do nothing;

insert into auth_users (id, login, password_hash, name, role)
values (
    '33333333-cccc-cccc-cccc-333333333333',
    'hr@diplomverify.ru',
    '$2a$10$7f2jyY18zCn5LeyJt2jP9Of0WixOkOTahKsBgd98iWlu9M6QXtMAy',
    'HR Demo',
    'hr'
)
on conflict (login, role) do nothing;
