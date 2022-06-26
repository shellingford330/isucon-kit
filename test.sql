SELECT p.id AS `id`, p.user_id AS `user_id`, p.body AS `body`, p.mime AS `mime`, p.created_at AS `created_at`, 
  u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at`
	FROM `posts` p 
	INNER JOIN `users` u ON u.id = p.user_id
	WHERE u.del_flg = 0
	ORDER BY p.created_at DESC
	LIMIT ?