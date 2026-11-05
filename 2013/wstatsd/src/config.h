#ifndef __config_h
#define __config_h

#include <ctype.h>
#include <stdio.h>
#include <assert.h>

#include <string>
#include <stack>
#include <map>

#include "logger.h"
extern Logger *log;

using namespace std;

class ConfigSection
{
protected:
	map<string, string> values;
	map<string,ConfigSection *> sections;
public:
	ConfigSection() { }
	~ConfigSection() { }

	void addValue(string key, string val)
	{
		values[key] = val;
	}

	ConfigSection *addSection(string key) {
		ConfigSection *ns = new ConfigSection;
		sections[key] = ns;
		return ns;
	}

	bool isValue(string key) {
		map<string,string>::iterator victim = values.find(key);
		if (victim == values.end()) {
			return false;
		}
		return true;
	}

	bool isSection(string key) {
		map<string,ConfigSection*>::iterator victim;
		victim = sections.find(key);
		if (victim == values.end()) {
			return false;
		}
		return true;
	}

	const char *get(string key, string def="") {
		map<string,string>::iterator victim = values.find(key);
		if (victim == values.end()) {
			if(def.compare("")==0) {
				return NULL;
			}
			return def.c_str();
		}
		return values[key].c_str();
	}

	string get_string(string key, string def="") {
		map<string,string>::iterator victim = values.find(key);
		if (victim == values.end()) {
			return def;
		}
		return values[key];
	}
};

class ConfigFile : public ConfigSection
{
protected:
	_IO_FILE *f;

public:

	stack<ConfigSection *> sources;
	stack<ConfigSection *> indexes;

	ConfigFile();
	bool load(char *);
	bool logError(int, int, char *);
};

#endif
