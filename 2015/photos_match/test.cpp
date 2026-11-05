#include <stdio.h>
#include "imagejpeg.h"

int main()
{
	ImageJpeg *img, *hist;

	img = new ImageJpeg;
	hist = new ImageJpeg;

	bool ret = img->load("database/1004_pb100177.jpg");

	hist->create(256,256);
	

	bool hist = img->histogram("test.hist");

	if (ret) {
		printf("Hello world!\n");
	} else {
		printf("Goodbye cruel world!\n");
	}
}
