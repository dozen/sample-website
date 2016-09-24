DROP TABLE IF EXISTS stars;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
id      INT             NOT NULL PRIMARY KEY,
name    varchar(255)    NOT NULL
);

CREATE TABLE articles(
id      INT             NOT NULL PRIMARY KEY,
title   VARCHAR(255)    NOT NULL,
user_id INT             NOT NULL,
content TEXT            NOT NULL,
FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE stars (
id          INT             NOT NULL PRIMARY KEY,
article_id  INT             NOT NULL,
user_id     INT             NOT NULL,
FOREIGN KEY (article_id)    REFERENCES articles(id),
FOREIGN KEY (user_id)       REFERENCES users(id)
);
