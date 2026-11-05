#include <stdarg.h>
#include "logger.h"

Logger::Logger(_IO_FILE *fd)
{
	this->f = fd;
}

Logger::~Logger()
{
	this->f = NULL;
}

bool Logger::set(char *filename)
{
	_IO_FILE *fn;
	fn = fopen(filename,"ab");
	if (!fn) {
		return false;
	} else {
		this->f = fn;
	}
	return true;
}

void Logger::output(char *msg, ...)
{
	char sBuf[2048];
	va_list ap;
	va_start(ap, msg);
	vsnprintf( sBuf, sizeof(sBuf), msg, ap);
	va_end(ap);

	fprintf(this->f, sBuf);
}
