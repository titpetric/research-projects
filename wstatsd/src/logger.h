#ifndef __logger_h
#define __logger_h

#include <stdio.h>
#include <stdlib.h>

class Logger
{
protected:
	_IO_FILE *f;

public:
	Logger(_IO_FILE *fd);
	~Logger();

	bool set(char *filename);

	void output(char *msg, ...);
//	void output(char *msg, bool newline=true);
//	void output(int msg, bool newline=true);
};

#endif
