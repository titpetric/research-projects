#ifndef __imagejpeg_h
#define __imagejpeg_h

#include <string>

using namespace std;

class ImageJpeg
{
protected:
	_IO_FILE *f;

	float normalize(int *src, unsigned short *dest, unsigned short cap);
	void destroy();

	bool loaded;

public:
	int height, width;
	unsigned char *data;

	ImageJpeg();
	~ImageJpeg();

	bool create(int width, int height);

	bool load(string filename);
	bool save(string filename, int quality=70);

	bool histogram(string filename);

	void buildHistogram();
};

#endif
