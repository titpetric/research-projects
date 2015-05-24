#ifndef __histogram_h_
#define __histogram_h_

#include <string>

typedef struct HistogramFile
{
	short width;
	short height;

	int bits_per_color;

	void *red, *green, *blue;
};

class Histogram
{
	private:
		_IO_FILE *f;

		bool loaded;

		HistogramFile my;
		HistogramFile comp;

	public:

		bool create(int bytes_per_channel, int bits_per_color);
		bool load(string filename);
		int compare(string dest);
		int compare(string src, string dest);
		bool save(string filename);
		int normalize(int bits_per_color);
};

#endif