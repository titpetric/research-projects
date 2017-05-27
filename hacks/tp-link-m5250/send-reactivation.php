#!/usr/bin/env php
<?php

mb_internal_encoding("utf-8");
error_reporting(E_ALL ^ E_NOTICE);

include("vendor/autoload.php");
include("bootstrap.php");

$login = Router::Login();

$previd = false;

do {
	$messages = Router::MessageInbox($login);
	$first = reset($messages);

	echo "Last message: " . print_r($first, true) . "\n";

	$message = Router::MessageRead($login, $first['id']);
	if ($previd === false || $previd !== $first['id']) {
		$previd = $first['id'];
		echo "\n";
	} else {
		if ($previd === $first['id']) {
			sleep(1);
			echo ".";
			continue;
		}
	}

	$lines = array_map("trim", explode("\n", $message['message']));

	print_r($lines);

	if (stripos($message['message'], "Opcija Internet Dan je deaktivirana") !== false) {
		Router::MessageSend($login, $message['from'], "internet dan");
	}

	if (stripos($message['message'], "Za potvrdu aktivacije opcije Internet Dan") !== false) {
		Router::MessageSend($login, $message['from'], "DA");
		break;
	}

} while (true);