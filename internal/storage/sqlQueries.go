package storage

var queryCreateUsers string = `CREATE TABLE IF NOT EXISTS public.users
(
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    login text NOT NULL,
    password text NOT NULL,
    salt text NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uk_login UNIQUE (login)
)
WITH (
    OIDS = FALSE
);`
var queryCreateOrders string = `CREATE TABLE IF NOT EXISTS public.orders
(
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    order_num bigint NOT NULL,
    user_id uuid NOT NULL,
    status smallint NOT NULL DEFAULT 0,
    accrual bigint,
    uploaded_at timestamp with time zone NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (id),
    CONSTRAINT uk_order_num UNIQUE (order_num),
    CONSTRAINT fk_users_id 
		FOREIGN KEY (user_id)
        REFERENCES public.users (id)
)
WITH (
    OIDS = FALSE
);`

// order_num can be absent in "orders" table
var queryCreateWithdrawals string = `CREATE TABLE IF NOT EXISTS public.withdrawals
(
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    user_id uuid NOT NULL,
    order_num bigint NOT NULL,
    sum bigint NOT NULL,
    processed_at timestamp with time zone NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (id),
    CONSTRAINT fk_users_id 
		FOREIGN KEY (user_id)
        REFERENCES public.users (id)
)
WITH (
    OIDS = FALSE
);`
