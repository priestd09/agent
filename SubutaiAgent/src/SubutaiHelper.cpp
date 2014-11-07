/**
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 *    @copyright 2014 Safehaus.org
 */
/**
 *  @brief     SubutaiHelper.cpp
 *  @class     SubutaiHelper.cpp
 *  @details   SubutaiHelper Class defines the helper methods..
 *  @author    Mikhail Savochkin
 *  @author    Ozlem Ceren Sahin
 *  @version   1.1.0
 *  @date      Oct 31, 2014
 */
#include "SubutaiHelper.h"
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include <iostream>

using namespace std;


/**
 *  \details   This method designed for Typically conversion from integer to string.
 */
string SubutaiHelper::toString(int intcont)
{		//integer to string conversion
    ostringstream dummy;
    dummy << intcont;
    return dummy.str();
}


/*
 * \details split string by delimeter
 *
 */
vector<string> SubutaiHelper::runAndSplit(char* cmd, char* type, char* delimeter)
{
	return splitResult(execCommand(cmd,type), delimeter);
}


/*
 * \details execute a terminal command. type = r, w, rw
 *
 */
string SubutaiHelper::execCommand(char* cmd, char* type) {
    FILE* pipe = popen(cmd, type);
    if (!pipe) return "ERROR";
    char buffer[128];
    string result = "";
    while(!feof(pipe)) {
        if(fgets(buffer, 128, pipe) != NULL)
            result += buffer;
    }
    pclose(pipe);
    return result;
}

/*
 * \details split string by delimeter
 *
 */
vector<string> SubutaiHelper::splitResult(string list, char* delimeter) {
    vector<string> tokens;
    size_t pos = 0;
    std::string token;
    while ((pos = list.find(delimeter)) != std::string::npos) {
        token = list.substr(0, pos);
        tokens.push_back(token);
        list.erase(0, pos + 1);
    }
    return tokens;
}
