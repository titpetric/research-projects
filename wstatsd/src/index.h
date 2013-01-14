#ifndef __index_h
#define __index_h

#include <map>
#include <string>

using namespace std;

class Index
{
protected:
	string name;
	map<string,string> definition;
public:
	Index();
	~Index();

	void addField(string keyname, string type) {
		definition[keyname] = type;
	}

	void setName(string sName) {
		name = sName;
	}
};


#endif
