#!/usr/bin/php
<?php

if ($argc < 3) {
	echo "Usage: ".$argv[0]." [server1] [server2]\n";
	die;
}

function getline()
{
        $line = "";
        $f = @fopen("php://stdin","r");
        if ($f) {
                $line = fgets($f);
                fclose($f);
        }
        return trim($line);
}


$server1_dpkg = $server2_dpkg = array();
exec("ssh ".$argv[1]." dpkg -l | grep ^ii | awk '{print $2 \" \" $3}'", $server1_dpkg);
exec("ssh ".$argv[2]." dpkg -l | grep ^ii | awk '{print $2 \" \" $3}'", $server2_dpkg);

$server1_dpkg = array_slice($server1_dpkg, 2);
$server2_dpkg = array_slice($server2_dpkg, 2);

$server1 = $server2 = array();
foreach ($server1_dpkg as $row) {
	$row = explode(" ", $row, 2);
	$server1[$row[0]] = $row[1];
}
foreach ($server2_dpkg as $row) {
	$row = explode(" ", $row, 2);
	$server2[$row[0]] = $row[1];
}

$server1_missing = $server1_version = array();
echo "server1 missing (".$argv[1]."): ";
foreach ($server1 as $k=>$v) {
	if (!isset($server2[$k])) {
		echo $k." ";
	}
}
echo "\n";
echo "server1 mismatch:\n";
foreach ($server1 as $k=>$v) {
	if (isset($server2[$k]) && $server2[$k]!=$v) {
		echo $k." ".$server2[$k]."!=".$v."\n";
	}
}
echo "\n";

echo "server2 (".$argv[2].") missing: ";
foreach ($server2 as $k=>$v) {
	if (!isset($server1[$k])) {
		echo $k." ";
	}
}
echo "\n";
echo "server2 mismatch:\n";
foreach ($server2 as $k=>$v) {
	if (isset($server1[$k]) && $server1[$k]!=$v) {
		echo $k." ".$server1[$k]."!=".$v."\n";
	}
}
echo "\n";

//var_dump($server1, $server2);