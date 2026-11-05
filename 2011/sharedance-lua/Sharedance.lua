--
-- Sharedance LUA client implementation
--
-- A very limited implementation of sharedance client, supporting only
-- the fetch command, for keys smaller than 255 bytes
--
-- Author: Tit Petric <tit.petric@monotek.net>
-- License: http://creativecommons.org/licenses/by-sa/3.0/
--


module('Sharedance', package.seeall);

require('socket');

local DEFAULT_HOST = "127.0.0.1";
local DEFAULT_PORT = 1042;
local DEFAULT_TIMEOUT = 10;

local host, port;

local function _connect()
	return socket.connect(host, port);
end

local function get(cache, key)
	local len = string.len(key);
	local request = 'F' .. string.char(0) .. string.char(0) .. string.char(0);
	if (len > 255) then
		return;
	end
	request = request .. string.char( len );
	request = request .. key;

	local conn = _connect();

	conn:send(request);
	local line, err = conn:receive("*a")
	conn:close();

	if not err then return line end
end

function Connect(hostname, portnumber)
	host = hostname;
	port = portnumber;
	if type(host) == 'table' then
		port = host[2];
		host = host[1];
	end
	if type(host) == 'number' then
		port = host;
		host = DEFAULT_HOST;
	end
	if host == nil then
		host = DEFAULT_HOST;
	end
	if port == nil then
		port = DEFAULT_PORT;
	end


	local cache = {
		get = get
	};

	return cache;
end

--
-- Usage:
--
-- require("Sharedance")
--
-- sd = Sharedance.Connect();
-- sd:get("key_name");
--
-- sd = Sharedance.Connect("hostname");
-- sd:get("key_name");
--
-- sd = Sharedance.Connect("hostname", 1234);
-- sd:get("key_name");
--
-- sd = Sharedance.Connect(1234);
-- sd:get("key_name");
--