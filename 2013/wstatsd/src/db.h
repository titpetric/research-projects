#ifndef __db_h
#define __db_h

#include <stdlib.h>
#include <string.h>
#ifdef HAVE_MYSQL
#include <mysql.h>
#endif

#include <string>
#include <stack>

using namespace std;

#include "logger.h"
extern Logger *log;

#ifdef HAVE_MYSQL
typedef struct tQuery
{
	MYSQL_RES	*m_result;
	MYSQL_FIELD	*m_fields;
	MYSQL_ROW	 m_row;
};
#endif

class MySQLConnection
{
protected:

#ifdef HAVE_MYSQL
	MYSQL		*m_conn;

	MYSQL_RES	*m_result;
	MYSQL_FIELD	*m_fields;
	MYSQL_ROW	 m_row;

	stack<tQuery*>	 m_query_stack;
#endif

	bool connected;

public:
	MySQLConnection();
	~MySQLConnection();

	bool connect(const char *h, const char *db, const char *user, const char *pass, const char *charset);

	int num_fields();

	const char *column(int i);
	const char *field(const char *field);
	const char *field_name(int i);

	bool query(const char *);
	bool query(string);

	bool fetch();
	void free_result();
	void close();

};

#endif
