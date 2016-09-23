<?php

class monosvn
{
	var $_settings, $_reports;

	function load($config)
	{
		if ($data = @simplexml_load_file($config)) {
			if (!isset($data->settings,$data->reports,$data->reports->report)) {
				return false;
			}
			$this->_settings = (array)$data->settings;
			$this->_settings['check-syntax'] = array_map("trim",explode(",",$this->_settings['check-syntax']));
			$this->_settings['ignore-files'] = array_map("trim",explode(",",$this->_settings['ignore-files']));
			$this->_reports = array();
			foreach ($data->reports->report as $report) {
				$this->_reports[] = (array)$report;
			}
			return true;
		}
		return false;
	}

	function error($message)
	{
		echo "\n".rtrim($message)."\n";
		$f = fopen("php://stderr","w");
		fwrite($f, "\n".rtrim($message)."\n");
		fclose($f);
		exit(1);
	}
}

$monosvn = new monosvn;
