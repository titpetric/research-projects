#!/usr/bin/env php
<?php

mb_internal_encoding("utf-8");
error_reporting(E_ALL ^ E_NOTICE);

include("vendor/autoload.php");
include("bootstrap.php");

$login = Router::Login();

Router::MessageSend($login, $argv[1], $argv[2]);
