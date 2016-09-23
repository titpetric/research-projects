<?php

chdir(dirname(__FILE__));

include("include/class.diff.php");
include("include/class.lint.php");
include("include/class.monosvn.php");

include("include/class.minitpl.php");

$monosvn->load("conf/monosvn.xml");

$actions = array("post-commit","pre-commit");

if (!isset($argv[1]) || !in_array($argv[1],$actions) || count($argv)!=4) {
	$monosvn->error("Usage: ".basename(__FILE__)." [ 'pre-commit' \$REPOS \$TXN | 'post-commit' \$REPOS \$REV ]");
}

switch ($argv[1]) {
	case "post-commit":
		$svn_repository = $argv[2];
		$svn_revision = $argv[3];

		$row = array();
		$row["revision"] = $svn_revision;
		$row["repository"] = $svn_repository;
		$row["author"] = trim(shell_exec('/usr/bin/svnlook author -r "'.$svn_revision.'" "'.$svn_repository.'"'));
		$row["modified_files"] = trim(shell_exec('/usr/bin/svnlook changed -r "'.$svn_revision.'" "'.$svn_repository.'"'));
		$row["modified_dirs"] = trim(shell_exec('/usr/bin/svnlook dirs-changed -r "'.$svn_revision.'" "'.$svn_repository.'"'));
		$row["log_message"] = trim(shell_exec('/usr/bin/svnlook log -r "'.$svn_revision.'" "'.$svn_repository.'"'));
		$row["log_datestamp"] = date("Y-m-d H:i:s");

		list($subject,$body) = $diff->get($row, $monosvn->_settings);

		$subject = "#".str_pad($svn_revision,5,'0',STR_PAD_LEFT).": [".$row['author']."] ".$subject;

		$headers = array();
		$headers[] = "From: ".$monosvn->_settings['from-name']." <".$monosvn->_settings['from-email'].'>';
		$headers[] = "Content-type: text/html; charset=utf-8";
		$headers[] = "X-Mailer: PHP/".phpversion();

		$destinations = array();
		foreach ($monosvn->_reports as $report) {
			$send = false;
			if (isset($report['@attributes']['user']) && $report['@attributes']['user']==$row['author']) {
				$send = true;
			}
			if (isset($report['@attributes']['location'])) {
				$dirname = $report['@attributes']['location'];
				if (in_array($dirname,array("*","/"))) {
					$send = true;
				} else {
					$dirname = ltrim($dirname,"/");
					$dirs_changed = explode("\n", $row['modified_dirs']);
					foreach ($dirs_changed as $k=>$v) {
						if ($v!='/') {
							$v = ltrim($v,"/");
							if (substr($v,0,strlen($dirname))==$dirname) {
								$send = true;
							}
						}
					}
				}
			}
			if ($send) {
				$emails = is_array($report['email']) ? $report['email'] : array($report['email']);
				foreach ($emails as $email) {
					if (!in_array($email,$destinations)) {
						$destinations[] = $email;
					}
				}
			}
		}

		$tpl = new minitpl;
		$tpl->load("monosvn_email.tpl");
		$tpl->assign("body", $body);
		$tpl->assign($diff->get_diff($row));
		$tpl->assign("settings", $monosvn->_settings);
		$tpl->assign($row);

		$contents = $tpl->get();

		foreach ($destinations as $email) {
			mail($email, $subject, $contents, implode("\r\n",$headers));
		}
	break;
	// Check files to be commited pre-commit
	case "pre-commit":
		$svn_repository = $argv[2];
		$svn_transaction = $argv[3];

		// don't allow commits with no message
		$look_commitreason = shell_exec('/usr/bin/svnlook log -t "'.$svn_transaction.'" "'.$svn_repository.'"');
		if (trim($look_commitreason)=='') {
			$monosvn->error("Commit failed! Please enter a commit reason.");
		}

		// can override syntax check with ".nosyntax" in commit message
		$syntax_check = (strpos($look_commitreason,".nosyntax")===false);

		// check syntax of defined file extensions
		if ($syntax_check && !empty($monosvn->_settings['check-syntax'])) {
			$look_changed = explode("\n",trim(shell_exec('/usr/bin/svnlook changed -t "'.$svn_transaction.'" "'.$svn_repository.'"')));
			foreach ($look_changed as $filename) {
				// dont lint deleted files
				if ($filename{0}=="D") {
					continue;
				}
				// strip status to get real filename
				$filename = trim(strstr($filename," "));
				foreach ($monosvn->_settings['check-syntax'] as $match) {
					if (fnmatch($match, $filename)) {
						$extension = substr($filename,1+strrpos($filename,"."));
						$lint_method = "check_".$extension;
						// check with given lint method for arbitrary extensions
						if (method_exists($lint, $lint_method)) {
							$tmpfname = tempnam("/tmp", "SVN");
							shell_exec('/usr/bin/svnlook cat -t "'.$svn_transaction.'" "'.$svn_repository.'" "'.$filename.'" > '.$tmpfname);
							$errors = $lint->$lint_method($filename,$tmpfname);
							unlink($tmpfname);
							if (!empty($errors)) {
								$monosvn->error(implode("\n",$errors)."\n -- you can use '.nosyntax' in the commit message to skip syntax checking\n");
							}
						}
						break;
					}
				}
				foreach ($monosvn->_settings['ignore-files'] as $match) {
					if (fnmatch($match, $filename)) {
						$monosvn->error("Can't commit filename '".$filename."', because we deny commits matching '".$match."'");
					}
				}
			}
		}
	break;
}
