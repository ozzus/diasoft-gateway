insert into auth_users (id, login, password_hash, name, role, organization_code, university_id)
values (
    '44444444-dddd-dddd-dddd-444444444444',
    'BMSTU',
    '$2a$10$7f2jyY18zCn5LeyJt2jP9Of0WixOkOTahKsBgd98iWlu9M6QXtMAy',
    'BMSTU Operator',
    'university',
    'BMSTU',
    '22222222-2222-2222-2222-222222222222'
)
on conflict (login, role) do nothing;

insert into auth_users (id, login, password_hash, name, role, organization_code, university_id)
values (
    '55555555-eeee-eeee-eeee-555555555555',
    'MSU',
    '$2a$10$7f2jyY18zCn5LeyJt2jP9Of0WixOkOTahKsBgd98iWlu9M6QXtMAy',
    'MSU Operator',
    'university',
    'MSU',
    '33333333-3333-3333-3333-333333333333'
)
on conflict (login, role) do nothing;
