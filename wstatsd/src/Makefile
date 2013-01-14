wstatsd: wstatsd.cpp config.cpp logger.cpp db.cpp index.cpp wstatsd.h config.h logger.h db.h index.h
	g++ wstatsd.cpp config.cpp logger.cpp db.cpp index.cpp -DHAVE_MYSQL -I/usr/include/mysql -L/usr/lib/mysql -lmysqlclient -lz -Wall -o wstatsd
debug: wstatsd.cpp config.cpp logger.cpp db.cpp index.cpp wstatsd.h config.h logger.h db.h index.h
	g++ wstatsd.cpp config.cpp logger.cpp db.cpp index.cpp -DHAVE_MYSQL -I/usr/include/mysql -L/usr/lib/mysql -lmysqlclient -lz -Wall -g -D__DEBUG__ -o wstatsd
	./wstatsd -s > .debug
clean:
	rm wstatsd

