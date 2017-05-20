<?php

class Router
{
	public static function Login()
	{
		if (file_exists("/tmp/router_session")) {
			return file_get_contents("/tmp/router_session");
		}

		if (!file_exists("/tmp/router_login")) {
			$curl = new \Curl;
			$curl->headers['Cookie'] = "Authorization=Basic%20YWRtaW46YWRtaW4%3D; subType=pcSub; TPLoginTimes=1";
			$res = $curl->get("http://192.168.0.1/loginRpm/pcLoginIndex.htm?login=1");
			file_put_contents("/tmp/router_login", $res->body);
		}

		$body = file("/tmp/router_login");
		foreach ($body as $line) {
			if (strpos($line, "session_id") !== false) {
				$line = explode("=", $line);
				$session_id = substr(trim($line[1]), 1, -2);
				file_put_contents("/tmp/router_session", $session_id);
			}
		}
		return $session_id;
	}

	public static function MessageInbox($session)
	{
		$curl = new \Curl;
		$curl->headers['Cookie'] = "Authorization=Basic%20YWRtaW46YWRtaW4%3D; subType=pcSub; TPLoginTimes=1";
		$res = $curl->get("http://192.168.0.1/userRpm/smsBox.htm?sms_box=2&page=1&session_id=" . $session);
		$matches = array();
		preg_match("/var smsEntryPara.+ \);/sm", $res->body, $matches);
		$data = explode("\n", $matches[0], 3);
		$data = '[' . trim(trim($data[1]), ',') . ']';
		$messages = array_chunk(json_decode($data, true), 5);
		foreach ($messages as $k => $v) {
			$messages[$k] = array_combine(array("id", "read", "date", "from", "message"), $v);
		}
		return $messages;
	}

	public static function MessageRead($session, $id)
	{
		$curl = new \Curl;
		$curl->headers['Cookie'] = "Authorization=Basic%20YWRtaW46YWRtaW4%3D; subType=pcSub; TPLoginTimes=1";
		$res = $curl->get("http://192.168.0.1/userRpm/smsSingle.htm?parent_path=smsBox.htm&page=1&sms_box=2&sms_index=" . $id . "&session_id=" . $session);
		preg_match("/var sms.+ \);/sm", $res->body, $matches);
		$data = explode("\n", $matches[0], 3);
		$data = explode(', "', $data[1]);
		$data = implode(', "', array_slice($data, 0, 4));
		$data = '[' . trim(trim($data), ',') . ']';
		$messages = json_decode($data, true);
		return array_combine(array("unknown", "from", "date", "message", "number"), $messages);
	}

	public static function MessageSend($session, $to, $message)
	{
		$curl = new \Curl;
		$curl->headers['Cookie'] = "Authorization=Basic%20YWRtaW46YWRtaW4%3D; subType=pcSub; TPLoginTimes=1";
		$time = date("Y,n,j,G,").intval(date("i")).",".intval(date("s"));
		$res = $curl->get("http://192.168.0.1/userRpm/smsSingle.htm?parent_path=&sms_index=0&numInputBoxId=" . $to . "&contentAreaId=" . $message . "&session_id=496795499&code_type=0&sendBtnId=1&sentTime=" . $time);
		echo "Sent message to $to: $message\n";
	}
}
