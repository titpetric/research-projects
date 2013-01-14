// $Id: $

#include <time.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include "wstatsd.h"

Logger *log;
ConfigFile *config;

int main(int argc, char **argv)
{
	log = new Logger( stdout );
	log->output( WSTATSD_BANNER );
	if (argc<=1) {
		log->output("\nProgram options:\n");
		log->output("================\n");
		log->output(" -c config.cnf        Specify config file (default = wstatsd.conf)\n");
		log->output(" -l logfile.log       Specify log file (default = standard out, wstatsd.log)\n");
		log->output(" -t seconds           Specify status report delay in seconds (default = off)\n");
		log->output(" -s                   Start wstatsd in foreground mode.\n");
		log->output(" -d                   Daemonize wstatsd.\n\n");
		return 0;
	}

	#define OPT(_a1) else if ( !strcmp(argv[i],_a1) )

	static char * wsConfName = "wstatsd.conf";
	static char * wsLogName = "wstatsd.log";
	bool wsDaemon = false;
	int wsReportTimeout = 0;

	int i;
	for (i=1; i<argc; i++) {
		if (argv[i][0] == '-') {

			if (false);
			// single switches
			OPT("-d")	wsDaemon   = true;
			OPT("-s")	wsDaemon   = false;
			else if (i+1 >= argc) break;
			// switch + parameter
			OPT("-c")	wsConfName = argv[++i];
			OPT("-l")	wsLogName  = argv[++i];
			OPT("-t")	wsReportTimeout = atoi(argv[++i]);
		}
	}

	config = new ConfigFile;
	config->load(wsConfName);

	if (wsDaemon || (strcmp(wsLogName,"wstatsd.log") != 0)) {
		if (wsDaemon) {
			log->output("Opening log file and going into daemon mode.\n");
			log->output("============================================\n");
		} else {
			log->output("Opening log file and staying in the foreground.\n");
			log->output("===============================================\n");
		}
		if (!log->set(wsLogName)) {
			log->output("Failed opening log file (permission denied, disk full?)\n");
			log->output("Staying with standard console output, deamonize = off.\n");
			log->output("======================================================\n");
			wsDaemon = false;
		}
	} else {
		log->output("Running in verbose mode (no log file, staying in foreground).\n");
		log->output("=============================================================\n");
	}

	if (wsDaemon) {
		int fd;
		switch (fork()) {
			case -1:
				log->output("Cant start daemon mode. Exiting.\n");
				return 1;
				break;
			case 0: break;
			default:
				_exit(EXIT_SUCCESS);
		}
		if (setsid() == -1) {
			log->output("Cant start daemon mode. Exiting.\n");
			return 1;
		}
		if ((fd = open("/dev/null", O_RDWR, 0)) != -1) {
			(void)dup2(fd, STDIN_FILENO);
			(void)dup2(fd, STDOUT_FILENO);
			(void)dup2(fd, STDERR_FILENO);
			if (fd > STDERR_FILENO)
				(void)close(fd);
		}
		log->output("Started daemon mode. Sleeping for 5 secs.\n");
		sleep(5);
	}

	log->output("Starting up wstatsd.\n");
	if (wsReportTimeout>0) {
		log->output("Status reporting is set to %d seconds.\n", wsReportTimeout);
	}

	// set up timers, network threads, bla bla bla bla.

	map<string,MySQLConnection *> sources_mysql;
	map<string,Index *> indexes;

	map<string,ConfigSection *> sources_config;

	ConfigSection *source_config, *config_tmp;
	MySQLConnection *mysql_tmp;
	Index *index_tmp;
	const string source_name;

	#define STACK_FOREACH(x,y) for (int c = x.size(), i=0; i<c; i++) { y=x.top(); x.pop();

	STACK_FOREACH(config->sources, source_config)
		if (source_config->isValue("name")) {
			mysql_tmp = new MySQLConnection;
			if (!mysql_tmp->connect(
				source_config->get("hostname",source_config->get_string("host","localhost")),
				source_config->get("database",source_config->get_string("db","wstatsd")),
				source_config->get("username",source_config->get_string("user","wstatsd")),
				source_config->get("password",source_config->get_string("pass","wstatsd")),
				source_config->get("charset","utf-8"))) {
					log->output("Connection to source [%s] failed.\n", source_config->get("name"));
					return -1;
			}
			sources_mysql[ source_config->get("name") ] = mysql_tmp;
			log->output("Added connection [%s] sucessfully.\n", source_config->get("name"));
		}
	}

	STACK_FOREACH(config->indexes, source_config)
		if (source_config->isValue("name") && source_config->isSection("sql_type")) {
			index_tmp = new Index;
			index_tmp->setName(source_config->get("name"));

			config_tmp = source_config->getSection("sql_type");

			indexes[ source_config->get("name") ] = index_tmp;
			log->output("Added index [%s] sucessfully.\n", source_config->get("name"));
		}
	}

	#undef STACK_FOREACH

	if (mysql_tmp->query("show tables;")) {
		int cols = mysql_tmp->num_fields(), i=0;
		log->output("Got %d columns\n", cols);
		for (i=0;i<cols;i++) {
			log->output("Got column [%s]\n", mysql_tmp->field_name(i));
		}
		i=0;
		while (mysql_tmp->fetch()) {
			log->output("Table [%s] Fields: ", mysql_tmp->column(0));

			string sql;

			sql = "desc ";
			sql += mysql_tmp->column(0);
			sql += ';';

			if (mysql_tmp->query(sql)) {
				while (mysql_tmp->fetch()) {
					log->output("%s ",mysql_tmp->column(0));
				}
				log->output("\n");
				mysql_tmp->free_result();
			}
		}
	}
	mysql_tmp->free_result();

	#define MAP_FOREACH(x) for(map<string,MySQLConnection*>::iterator cc = x.begin(); cc!=x.end(); cc++)

	MAP_FOREACH(sources_mysql) {
		log->output("Closing mysql connection [%s]\n", cc->first.c_str());
		delete cc->second;
	}

	#undef MAP_FOREACH

	log->output("Exiting.\n");
	return 0;
}
