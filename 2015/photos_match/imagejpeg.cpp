#include <stdio.h>
#include <jpeglib.h>
#include <setjmp.h>
#include "imagejpeg.h"

ImageJpeg::ImageJpeg()
{
	this->loaded = false;
}

ImageJpeg::~ImageJpeg()
{
	this->destroy();
}

void ImageJpeg::destroy()
{
	if (this->loaded) {
		free(this->data);
		this->loaded = false;
	}
}

bool ImageJpeg::create(int width, int height)
{
	this->destroy();
	this->data = (unsigned char*)malloc(width * height * 3); // rgb
	if (this->data!=NULL) {
		memset((void*)this->data,255,(width*height*3));
		this->width = width;
		this->height = height;
		this->loaded = true;
	}
	return this->loaded;
}

struct my_error_mgr {
  struct jpeg_error_mgr pub;	/* "public" fields */

  jmp_buf setjmp_buffer;	/* for return to caller */
};

typedef struct my_error_mgr * my_error_ptr;

/*
 * Here's the routine that will replace the standard error_exit method:
 */

METHODDEF(void)
my_error_exit (j_common_ptr cinfo)
{
  /* cinfo->err really points to a my_error_mgr struct, so coerce pointer */
  my_error_ptr myerr = (my_error_ptr) cinfo->err;

  /* Always display the message. */
  /* We could postpone this until after returning, if we chose. */
  (*cinfo->err->output_message) (cinfo);

  /* Return control to the setjmp point */
  longjmp(myerr->setjmp_buffer, 1);
}

// @todo: consider moving this to histogram | common class?
float ImageJpeg::normalize(int *src, unsigned short *dest, unsigned short cap) {
	int max = 0;
	float factor = 0.0;
	int i;
	for (i=0;i<256;i++) {
		if (src[i] > max) {
			max = src[i];
		}
	}
	factor = (float)cap / (float)max;
	for (i=0;i<256;i++) {
		max = (int)((float)src[i] * factor);
		if (max > cap) {
			max = cap;
		}
		dest[i] = max;
	}
	return factor;
}

bool ImageJpeg::load(string filename)
{
	struct jpeg_decompress_struct cinfo;
	struct my_error_mgr jerr;

	JSAMPARRAY buffer;		/* Output row buffer */
	int row_stride;			/* physical row width in output buffer */
	int offs;

	// free up previous
	this->destroy();

	this->f = fopen(filename.c_str(), "rb");
	if (!this->f) {
		return false;
	}

	cinfo.err = jpeg_std_error(&jerr.pub);
	jerr.pub.error_exit = my_error_exit;
	/* Establish the setjmp return context for my_error_exit to use. */
	if (setjmp(jerr.setjmp_buffer)) {
		jpeg_destroy_decompress(&cinfo);
		this->destroy();
		fclose(this->f);
		return false;
	}


	jpeg_create_decompress(&cinfo);

	jpeg_stdio_src(&cinfo, this->f);

	(void) jpeg_read_header(&cinfo, TRUE);
	(void) jpeg_start_decompress(&cinfo);

	printf("Loading jpeg image %s, with size %d x %d x %d\n", filename.c_str(), cinfo.output_width, cinfo.output_height, cinfo.output_components);

	// we need a rgb image
	if (cinfo.output_components != 3) {
		jpeg_destroy_decompress(&cinfo);
		this->destroy();
		fclose(this->f);
		return false;
	}

	this->width = cinfo.output_width;
	this->height = cinfo.output_height;

	row_stride = cinfo.output_width * cinfo.output_components;

	data = (unsigned char *)malloc(row_stride * cinfo.output_height);
	if (data==NULL) {
		return false;
	}
	this->loaded = true;

	/* Make a one-row-high sample array that will go away when done with image */
	buffer = (*cinfo.mem->alloc_sarray) ((j_common_ptr) &cinfo, JPOOL_IMAGE, row_stride, 1);

	offs = 0;
	while (cinfo.output_scanline < cinfo.output_height) {
		(void)jpeg_read_scanlines(&cinfo, buffer, 1);
		memcpy(buffer[0], this->data+offs, row_stride);
		offs+=row_stride;
		//put_scanline_someplace(buffer[0], row_stride);
	}
	printf("Fetched %d bytes\n", offs);
	(void) jpeg_finish_decompress(&cinfo);
	jpeg_destroy_decompress(&cinfo);
	fclose(this->f);
	return true;
}

bool ImageJpeg::histogram(string filename)
{
	this->f = fopen(filename.c_str(),"wb");
	if (!this->f) {
		return false;
	}

	int red[256], green[256], blue[256];
	unsigned short usRed[256], usGreen[256], usBlue[256];
	int i=0;
	for (int y=0; y<this->height; y++) {
		for (int x=0; x<this->width; x++) {
			unsigned char r, g, b;
			r = data[i];
			g = data[i+1];
			b = data[i+2];
			red[r]++;
			green[g]++;
			blue[b]++;
			i+=3;
		}
	}
	float fRed, fGreen, fBlue;
	fRed = this->normalize(red, usRed, 0xffff);
	fGreen = this->normalize(green, usGreen, 0xffff);
	fBlue = this->normalize(blue, usBlue, 0xffff);

	#define PUT(y,x) fwrite(x, y, 1, this->f);

	PUT(sizeof(short), &this->width);
	PUT(sizeof(short), &this->height);
	PUT(sizeof(float), &fRed);
	PUT(sizeof(float), &fGreen);
	PUT(sizeof(float), &fBlue);

	PUT(256*sizeof(unsigned short), &usRed);
	PUT(256*sizeof(unsigned short), &usGreen);
	PUT(256*sizeof(unsigned short), &usBlue);

	#undef PUT

	fclose(this->f);

	return true;
}
