#include "db.h"

MySQLConnection::MySQLConnection() {
#ifdef HAVE_MYSQL
	// connection
	this->m_conn = NULL;
	// result cache
	this->m_result = NULL;
	this->m_fields = NULL;
#endif
	this->connected = false;
}

bool MySQLConnection::connect(const char *h, const char *db, const char *user, const char *pass, const char *charset)
{
#ifdef HAVE_MYSQL
	if (NULL == (this->m_conn = mysql_init(NULL))) {
		log->output("mysql_init() failed, exiting...\n");
		return false;
	}
#if MYSQL_VERSION_ID >= 50013
	/* in mysql versions above 5.0.3 the reconnect flag is off by default */
	my_bool reconnect = 1;
	mysql_options(this->m_conn, MYSQL_OPT_RECONNECT, &reconnect);
#endif
	if (!mysql_real_connect(this->m_conn, h, user, pass, db, 3306, NULL, 0)) {
		log->output("MySQL Connect Error: %s\n", mysql_error(this->m_conn));
		return false;
	}
	this->connected = true;
	return true;
#endif
	return false;
}

bool MySQLConnection::query(const char *sql)
{
#ifdef HAVE_MYSQL
	if (this->connected) {

		if (this->m_result!=NULL) {
			// we already have a running query, do a new one
			tQuery *tmp = new tQuery;
			#define MIRROR(x) tmp->x = this->x;
			MIRROR(m_result);
			MIRROR(m_fields);
			MIRROR(m_row);
			#undef MIRROR
			this->m_result = NULL;
			this->m_fields = NULL;
			this->m_query_stack.push(tmp);
		}

		if (mysql_query(this->m_conn, sql)) {
			log->output("Mysql Query Error: %s\n", mysql_error(this->m_conn));
			return false;
		}
		this->m_result = mysql_store_result(this->m_conn);
		return true;
	}
#endif
	return false;
}

bool MySQLConnection::query(string sql)
{
	return this->query(sql.c_str());
}

void MySQLConnection::free_result()
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		if (!this->m_result) return;
		mysql_free_result(this->m_result);
		this->m_result = NULL;
		this->m_fields = NULL;

		if (this->m_query_stack.size()>0) {
			tQuery *tmp = this->m_query_stack.top();
			this->m_query_stack.pop();
			#define MIRROR(x) this->x = tmp->x;
			MIRROR(m_result);
			MIRROR(m_fields);
			MIRROR(m_row);
			#undef MIRROR
			delete tmp;
		}
	}
#endif
}

int MySQLConnection::num_fields()
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		if (this->m_result != NULL) {
			return mysql_num_fields(this->m_result);
		}
	}
#endif
	return -1;
}

const char *MySQLConnection::field_name(int i)
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		if (!this->m_result) return NULL;
		if (!this->m_fields) this->m_fields = mysql_fetch_fields(this->m_result);
		return this->m_fields[i].name;
	}
#endif
	return NULL;
}

const char *MySQLConnection::column(int i)
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		if (!this->m_result) return NULL;
		return this->m_row[i];
	}
#endif
	return NULL;
}

/* * *
 * this probably isn't how you should do it, especially with hashes it should be more like return this->m_row["id"], et cetera.
 * * */

const char *MySQLConnection::field(const char *field)
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		int cols;
		if (!this->m_result) return NULL;
		if (!this->m_fields) this->m_fields = mysql_fetch_fields(this->m_result);
		cols = this->num_fields();
		if (cols!=-1) {
			for (int i=0;i<cols;i++) {
				if (strcmp(this->m_fields[i].name,field)==0) {
					return this->m_row[i];
				}
			}
		}
	}
#endif
	return NULL;
}

bool MySQLConnection::fetch()
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		if (!this->m_result) return false;
		m_row = mysql_fetch_row(this->m_result);
		return m_row!=NULL;
	}
#endif
	return false;
}

void MySQLConnection::close()
{
#ifdef HAVE_MYSQL
	if (this->connected) {
		mysql_close(this->m_conn);
		this->connected = false;
	}
#endif
}

MySQLConnection::~MySQLConnection()
{
	this->close();
}
