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

	if (in_array("3. Provjera stanja racuna", $lines)) {
		Router::MessageSend($login, $message['from'], "3");
	}

	if (in_array("1. Glavni racun", $lines)) {
		Router::MessageSend($login, $message['from'], "1");
	}

} while (true);