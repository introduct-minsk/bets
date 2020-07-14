create table users
(
    id      serial primary key,
    balance int  not null default 0,
    email   text NOT NULL unique
);

create table sources
(
    id    serial primary key,
    value text not null
);


create table bets
(
    external_id text primary key,
    user_id     int       not null references users (id),
    type        text      not null,
    amount      int       not null,
    source_type int       not null references sources (id),
    processed   bool      not null default false,
    created_at  timestamp not null default now()

);

insert into users (email)
values ('betplacer@gmail.com');

insert into sources (value)
values ('game'),
       ('server'),
       ('payment');