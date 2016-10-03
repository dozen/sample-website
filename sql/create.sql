DROP TABLE IF EXISTS followings;

DROP TABLE IF EXISTS favorites;

DROP TABLE IF EXISTS articles;

DROP TABLE IF EXISTS users;

CREATE TABLE users (
id      INT             NOT NULL AUTO_INCREMENT PRIMARY KEY,
name    varchar(255)    NOT NULL
)ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE articles (
id      INT             NOT NULL AUTO_INCREMENT PRIMARY KEY,
title   VARCHAR(255)    NOT NULL,
user_id INT             NOT NULL,
content TEXT            NOT NULL,
)ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE favorites (
id          INT             NOT NULL AUTO_INCREMENT PRIMARY KEY,
article_id  INT             NOT NULL,
user_id     INT             NOT NULL,
)ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE followings (
id              INT             NOT NULL AUTO_INCREMENT PRIMARY KEY,
from_id         INT             NOT NULL,
to_id           INT             NOT NULL,
)ENGINE=InnoDB DEFAULT CHARSET=utf8;
