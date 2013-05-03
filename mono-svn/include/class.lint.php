<?php

class lint
{
	function check_xml($filename, $tmpfname)
	{
		$errors = array();
		$xml_parser = xml_parser_create();
		xml_set_element_handler($xml_parser, array($this,"_check_xml_startElement"), array($this,"_check_xml_endElement"));
		if (!($fp = fopen($tmpfname, "r"))) {
			$errors[] = "lint::check_xml: Could not open XML input '".$tmpfname."'";
		} else {
			while ($data = fread($fp, 4096)) {
				if (!xml_parse($xml_parser, $data, feof($fp))) {
					$errors[] = sprintf("lint::check_xml: Error in %s, %s at line %d", $filename, xml_error_string(xml_get_error_code($xml_parser)), xml_get_current_line_number($xml_parser));
				}
			}
			xml_parser_free($xml_parser);
		}
		return $errors;
	}

	function _check_xml_startElement($parser, $name, $attrs) {}
	function _check_xml_endElement($parser, $name) {}

	function check_php($filename, $tmpfname)
	{
		$errors = array();
		$exec = array("/usr/bin/php5","/usr/bin/php","/usr/local/bin/php5","/usr/local/bin/php");
		foreach ($exec as $binary) {
			if (is_file($binary) && is_executable($binary)) {
				$p = popen($binary." -l ".$tmpfname." 2>&1", "r");
				while ($line = fgets($p)) {
					if (trim($line)!='') {
						$errors[] = trim(str_replace($tmpfname,$filename,$line));
						break;
					}
				}
				if (pclose($p)==0) {
					$errors = array();
				}
				return $errors;
			}
		}
		$errors[] = "lint::check_php: Can't find PHP binary to execute with (nonstandard location)";
		return $errors;
	}

	function check_phtml($filename, $tmpfname)
	{
		return $this->check_php($filename, $tmpfname);
	}
}

$lint = new lint;