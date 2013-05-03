<?php

class diff
{
	function print_files($header,$files,$length=60,$spacing="        ") {
		$ret = "";
		if (count($files)>0) {
			$ret = "<b>".$header."</b>\n";
			$line = '';
			foreach ($files as $k=>$v) {
				if (strlen($line.$v)<$length) {
					$line = trim($line.' '.$v);
				} else {
					$ret .= $spacing.$line."\n";
					$line = $v;
				}
			}
			$ret .= $spacing.$line."\n";
		}
		return $ret;
	}

	function count_lines($contents) {
		$minus = 0;
		$plus = 0;
		if (is_array($contents)) {
			foreach ($contents as $line) {
				switch (true) {
					case ($line{0}=='-' && $line{1}!='-' && $line{2}!='-'):	$minus++;
												break;
					case ($line{0}=='+' && $line{1}!='+' && $line{2}!='+'):	$plus++;
												break;
				}
			}
		}
		return array($minus,$plus);
	}

	function get_diff($row) {
		$retval = array();
		$retval['diff'] = explode("\n",trim(shell_exec('/usr/bin/svnlook diff -r '.$row['revision'].' '.$row['repository'])));
		list($retval['minus'],$retval['plus']) = $this->count_lines($retval['diff']);
		return $retval;
	}

	function get($row, $settings) {
		$modified_files = $row['modified_files'];
		$modified_dirs = $row['modified_dirs'];

		$look_changed = explode("\n",$modified_files);
		$adds = $dels = $mods = array();
		foreach ($look_changed as $k=>$v) {
			$temp = trim(strstr($v," "));
			if ($v{0}=='A') {
				$adds[] = $temp;
			} elseif ($v{0}=='D') {
				$dels[] = $temp;
			} else {
				$mods[] = $temp;
			}
			$look_changed[$k] = $temp;
			unset($temp);
		}
		// diff, minus, plus
		$diff = $this->get_diff($row);
		$subject = count($adds)." files added | ".count($mods)." files modified | ".count($dels)." files deleted (+".$diff['plus']." lines, -".$diff['minus']." lines)";
		$body = '<pre><b>SVN Commit Log / '.$row['author'].'@'.php_uname("n").'</b></pre>';
		$body .= "<pre>";
		$body .= $this->print_files("Added files:",$adds);
		$body .= $this->print_files("Modified files:",$mods);
		$body .= $this->print_files("Deleted files:",$dels);
		$body .= "<b>Log Message:</b>\n".htmlspecialchars($row['log_message'])."\n</pre>";
		return array($subject,$body);
	}
}

$diff = new diff;