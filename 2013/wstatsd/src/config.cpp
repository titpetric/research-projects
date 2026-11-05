#include "config.h"

enum eStates { STATE_TOP, STATE_SKIP_LINE, STATE_TYPE, STATE_SECTION, STATE_SECTION_VALUES, STATE_GROUP };

ConfigFile::ConfigFile() {}

inline bool isAlphaNum(int c)
{
        return isalpha(c) || isdigit(c) || c=='_';
}

bool ConfigFile::load(char *filename) {
	char sBuffer[256];
	char sToken[64];

	char sName[64], sValue[64];
	int iName=0, iValue=0;
	int token_state = 0; // 0 = sName, 1=sValue;

	eStates iState = STATE_TOP;
	stack<eStates> state_stack;

	ConfigSection *group=NULL, *section;
	stack<ConfigSection *> group_stack;

	int iToken=0;

	int pos_line = 0, pos_char = -1;

	char *p=NULL, *pe=NULL;

	log->output("loading config %s\n", filename);

	#define STACK_PUSH(_new) { state_stack.push(iState); iState = _new; }
	#define STACK_POP() { iState = state_stack.top(); state_stack.pop(); }
	#define STACK_BACK() { p--; if ( *p=='\n' ) pos_line--; }

	#define GROUP_PUSH(_name) { group_stack.push(group); group = group->addSection(_name); }
	#define GROUP_POP() { group = group_stack.top(); group_stack.pop(); }

	#define TOKEN_RESET() { iName = iValue = iToken = token_state = 0; memset((void*)sToken,0,sizeof(sToken)); memset((void*)sName,0,sizeof(sName)); memset((void*)sValue,0,sizeof(sValue)); }
	
	STACK_PUSH( STATE_TOP );

	this->f = fopen(filename,"rb");

	if (!this->f) {
		log->output("Can't open config file!\n");
		return false;
	}

	for (;;p++) {
		pos_char++;
		if (p>=pe) {
			int iLen = fread(sBuffer, sizeof(char), sizeof(sBuffer), this->f);
			if (iLen<=0) {
				break;
			}
			p = sBuffer;
			pe = sBuffer+iLen;
		}

		if (*p=='\n') {
			pos_line++;
			pos_char=-1;
			if (iState == STATE_SKIP_LINE) {
				STACK_POP();
				continue;
			}
		}
		if (iState == STATE_SKIP_LINE) {
			continue;
		}

		if (iState == STATE_TOP) {
			if (isspace(*p)) continue;
			if (*p=='#') {
				STACK_PUSH( STATE_SKIP_LINE );
				continue;
			}
			if (!isAlphaNum(*p)) {
				return this->logError(pos_line, pos_char, "Invalid token");
			}
			TOKEN_RESET();
			STACK_PUSH( STATE_TYPE );
			STACK_BACK();
			continue;
		}

		if (iState == STATE_TYPE) {
			if (!iToken && !isAlphaNum(*p)) {
				return this->logError(pos_line, pos_char, "Internal error, non-alpha in SECTION_NAME");
			}
			if (iToken == sizeof(sToken)) {
				return this->logError(pos_line, pos_char, "Token too long");
			}
			if (!isAlphaNum(*p)) {
				STACK_POP();

				if (strcasecmp(sToken,"source")==0) {
					section = new ConfigSection;
					this->sources.push(section);
					STACK_PUSH( STATE_SECTION );
					STACK_BACK();
					TOKEN_RESET();
					continue;
				}
				if (strcasecmp(sToken,"index")==0) {
					section = new ConfigSection;
					this->indexes.push(section);
					STACK_PUSH( STATE_SECTION );
					STACK_BACK();
					TOKEN_RESET();
					continue;
				}
				TOKEN_RESET();
				STACK_BACK();
				continue;
			}
			sToken[iToken++] = *p;
			continue;
		}


		if (iState == STATE_SECTION) {
			if (isspace(*p)) continue;
			if (isAlphaNum(*p)) {
				return this->logError(pos_line, pos_char, "section has alphanumeric garbage before begin!");
			}
			if (*p == '{') {
				STACK_POP();
				STACK_PUSH( STATE_SECTION_VALUES );
				token_state = 0;
				continue;
			}
			return this->logError(pos_line, pos_char, "expected '{' for section start, not found");
		}

		if (iState == STATE_SECTION_VALUES) {
			if (*p=='\n' || *p==';') {
				if (*p=='\n' && iName==0 && iValue==0) {
					continue;
				}
				if (token_state==1 || iValue==0) {
					section->addValue(sName,sValue);
					TOKEN_RESET();
					continue;
				} else {
					if (token_state==0 && iName==0) {
						return this->logError(pos_line, pos_char, "error, token in state 0, expected ':' not new line");
					}
				}
			}
			if (isspace(*p)) continue;
			if (*p=='#') {
				STACK_PUSH( STATE_SKIP_LINE );
				continue;
			}
			if (*p==':' && token_state==0) {
				token_state = 1;
				continue;
			}
			if (*p=='"' || *p=='\'') {
				continue;
			}
			if (*p=='{') {
				group = section->addSection( sName );
				GROUP_PUSH( sName );
				STACK_PUSH( STATE_GROUP );
				TOKEN_RESET();
				continue;
			}
			if (*p=='}') {
				STACK_POP(); // STATE_TOP
				continue;
			}
			if (!isAlphaNum(*p) && token_state==0) {
				return this->logError(pos_line, pos_char, "Invalid token in section");
			}
			if (token_state==0) {
				if (iName==sizeof(sName)) {
					return this->logError(pos_line, pos_char, "Too long token name");
				}
				sName[iName++] = *p;
			} else {
				if (iValue==sizeof(sValue)) {
					return this->logError(pos_line, pos_char, "Too long token value");
				}
				sValue[iValue++] = *p;
			}
			continue;
		}

		if (iState == STATE_GROUP) {
			if (*p=='\n' || *p==';') {
				if (*p=='\n' && iName==0 && iValue==0) {
					continue;
				}
				if (token_state==1 || iValue==0) {
					group->addValue(sName,sValue);
					TOKEN_RESET();
					continue;
				} else {
					if (token_state==0 && iName==0) {
						return this->logError(pos_line, pos_char, "error, token in state 0, expected ':' not new line");
					}
				}
			}
			if (isspace(*p)) continue;
			if (*p=='#') {
				STACK_PUSH( STATE_SKIP_LINE );
				continue;
			}
			if (*p==':' && token_state==0) {
				token_state = 1;
				continue;
			}
			if (*p=='"' || *p=='\'') {
				continue;
			}
			if (*p=='{') {
				GROUP_PUSH(sName);
				STACK_PUSH( STATE_GROUP );
				TOKEN_RESET();
				continue;
			}
			if (*p=='}') {
				GROUP_POP();
				STACK_POP(); // STATE_TOP
				TOKEN_RESET();
				continue;
			}
			if (!isAlphaNum(*p) && token_state==0) {
				return this->logError(pos_line, pos_char, "Invalid token in group");
			}
			if (token_state==0) {
				if (iName==sizeof(sName)) {
					return this->logError(pos_line, pos_char, "Too long token name in group");
				}
				sName[iName++] = *p;
			} else {
				if (iValue==sizeof(sValue)) {
					return this->logError(pos_line, pos_char, "Too long token value in group");
				}
				sValue[iValue++] = *p;
			}
			continue;
		}
		return this->logError(pos_line,pos_char,"Invalid state");
	}

	#undef STACK_PUSH
	#undef STACK_POP
	#undef STACK_BACK

	fclose(this->f);

	return true;
}

bool ConfigFile::logError(int pos_line, int pos_char, char *error_message) {
	fclose(this->f);
	log->output("Config error, line %d, char %d: %s\n", pos_line, pos_char, error_message);
	return false;
}
