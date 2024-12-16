package models

var InitialData = []string{
	// 默认admin
	`INSERT INTO  user  ( id, name, email, display, comment, type, deleted, admin, last_login, last_ip, create_time, update_time, password, salt ) VALUES (1,'admin','admin@gmail.com','管理员','',0,0,1,now(),'127.0.0.1',now(),now(),'e7cadd50397b88397045bf1b7f406b34dc8dc6b8f79d470c0a80cf7aad08690748bf5e6c2d0881bb8bb9c96045b08318fa2b','BZoWKqwaQ6');`,
}
