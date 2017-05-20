# TP-Link M5250 automation

I have this personal AP/travel router in Croatia, with a pre-paid sim card with daily
internet access. The web interface is *horrible*, built mostly in javascript. No API,
poor UX, whatever.

These scripts partially automate some common flows, mainly:

1. Login into the router,
2. Sendind SMS messages,
3. Reading SMS messages (inbox digest + individual messages).

I created some [simpa.hr](https://www.simpa.hr/tarifa/opcije/internet-opcije) automation.
Mainly I want to activate the daily option with scripts, and ideally, if the machine is online,
also schedule an internet disconnect + reactivation when the previous option expires.

Individual steps rely on SMS replies to drive automation further. If you have your
own M5250 device, you could automate something else based on SMS messages. If you're
fine with PHP, and such.

There's only a dependency on the shuber/curl HTTP client. Generally it's pretty simple,
most of the usable implementation is located in boostrap.php.