-- example HTTP POST script which demonstrates setting the
-- HTTP method, body, and adding a header

local charset = {}

-- qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM1234567890
for i = 48,  57 do table.insert(charset, string.char(i)) end
for i = 65,  90 do table.insert(charset, string.char(i)) end
for i = 97, 122 do table.insert(charset, string.char(i)) end

function string.random(length)
  math.randomseed(os.clock()^5)

  if length > 0 then
    return string.random(length - 1) .. charset[math.random(1, #charset)]
  else
    return ""
  end
end

request = function()
  wrk.method = "POST"
  wrk.body   = "email=" .. string.random(12) .. "&name=x&gitlab=" .. string.random(12)
  wrk.headers["Content-Type"] = "application/x-www-form-urlencoded"
  return wrk.format("POST", "/api/sign")
end

-- logfile = io.open("/tmp/wrk.log", "w");
-- local cnt = 0;
--
-- response = function(status, header, body)
--     logfile:write("status:" .. status .. "\n");
--     cnt = cnt + 1;
--     logfile:write("status:" .. status .. "\n" .. body .. "\n-------------------------------------------------\n");
-- end