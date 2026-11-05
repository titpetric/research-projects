#!/usr/bin/perl
#
# a simple DTD to HTML renderer
#
######################### 
# $Id: dtd2html.pl 111 2004-03-18 21:13:05Z steve $
#########################
# $Log$
# Revision 1.2  2004/03/18 21:13:05  steve
# improved regexps
#
# Revision 1.1  2004/01/20 17:27:33  steve
# imported from an old project
#
#########################

use strict;

use Getopt::Std;

our ( $opt_f, $opt_h, $opt_V );
getopts("fhV");

my $url = "http://staticfree.info/software/dtd2html/";
my ($version) = '$Revision: 111 $' =~ /Revision:\s*(.+)\$/;

format_usage() if $opt_f;
usage() if $opt_h;
die "$0 version: $version\n" if $opt_V;

#### end init

usage() unless @ARGV;

my $file = $ARGV[0];

my $debug = 0;
my %ents;
my @parsed = parse_file( $file );


use Data::Dumper;

print Dumper \@parsed if $debug;

print<<HTML;
<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">
<html>
  <head>
    <title>$file</title>
    <style type="text/css">
    code { color: green; }
    .required { font-weight: bold; }
    .implied  { font-weight: bold; color: #555; font-style: italic; }
    .empty, .empty * { font-weight: bold; color: #c00; }
    .footnote { font-size: small; font-style: italic; }
    </style>
  </head>
  <body>
  <!--toc-->
HTML

    print contents( \@parsed );

print<<END_HTML;
<p class="footnote">auto-generated from <a href="$file">$file</a> 
by <a href="$url">dtd2html.pl</a> version $version</p> 
</body>

END_HTML


# parses the file and returns an 2d array in the following form:
# ['comment', "comment text"]
# ['!ELEMENT', "element text"]

sub parse_file($){
    my ($file) = @_;

    open DTD, "<$file" or die "$file cannot be opened for reading: $!";


# current mkup
    my $mkup = "";
    my $mkup_body = "";
    my $comment = 0;

    my $unproc = "";

    my @parsed = ();

    while( my $line = <DTD> ){
	$unproc = $line;

	while( $unproc ){

	    print "$unproc\n" if $debug;

	    # comments
	    if( !$comment && $unproc =~ /^\s*\Q<!--\E(.*$)/sm ){
		$comment = 1;
		$unproc = $1;
		print "start comment\n" if $debug;

		# end comments
	    }elsif( $comment && $unproc =~ /^(.*)\Q-->\E(.*$)/sm  ){
		$comment = 0;
		$mkup_body .= $1;
		$unproc = $2;
		push @parsed, ['comment', $mkup_body];
		$mkup_body = "";
		print "end comment\n" if $debug;

		# markup that is not a comment
	    }elsif( !$comment && $unproc =~ /^\s*\<([^\s]+)(.*$)/sm ){
		$mkup = $1;
		$unproc  = $2;
		print "start mkup\n" if $debug;

		# end markup
	    }elsif( !$comment && $unproc =~ /^(.*)\>(.*$)/sm ){
		$mkup_body .= $1;
		$unproc = $2;
		push @parsed, [$mkup, $mkup_body];
		$mkup_body = "";
		$mkup = "";
		print "end mkup\n" if $debug;

		# stuff inside the <>
	    }elsif( $comment || $mkup ){
		$mkup_body .= $unproc;
		$unproc = "";

		# stuff outside the <>
	    }else{
		$unproc = "";
	    }

	    #<STDIN> if $debug == 2;
	}

    }
    return @parsed;
}

# looks through all the elements and makes a hash of arrays of things 
# a given element is inside. eg:
# $foo{'moo'} = ['cow', 'confused_chicken']
#
# the moo element is allowed in the cow or the confused_chicken
sub gen_rev_index($){
    my ( $parsed ) = @_;

    my %rev;

    foreach my $part (@$parsed){
	
	if( @$part[0] eq '!ELEMENT' ){
	    @$part[1] =~ /([\w.-]+)\s+(.+)/s;
	    my $element  = $1;
	    my $contents = $2;

	    # go through each of the children
	    while( $contents =~ /(\b(?!PCDATA\b)[\w.-]+\b)/g ){
		$rev{$1} = [] unless exists $rev{$1};
		push @{$rev{$1}}, $element;
	    }
	}
    }
    return \%rev;
}

# prints the contents of the dtd. pass in the parsed DTD
sub contents($){
    my ( $parsed ) = @_;

    my %rev = %{gen_rev_index( $parsed )};

    my $out = "";
    my %desc;
    my $mkup;

	my $prevelem = "<dummy>";

    # let the header be h1
    my $l = 1;
    foreach my $part ( @$parsed ){	
	if( @$part[0] eq 'comment' ){

	    @$part[1] = escaped( @$part[1] );

	    if( @$part[1] =~ /^\s*\w+\:\s*.+(\n|$)/ ){

		my $c = "";
		foreach my $line ( split /\n/, @$part[1] ){

		    if( $line eq "" ) {
			$desc{$c} .= "\n\n";
		    }elsif( $line =~ /\s*(\w+)\s+\-\s*(.+)$/ ){
			$c = "$mkup/$1";
			$desc{$c} = $2;
			
		    }elsif( $line =~ /^\s*(\w+)\:\s*(.+)$/ ){
			$mkup = $1;
			$desc{$mkup} = $2;
			$c = $mkup;

		    }else{
			$desc{$c} .= $line."\n";
		    }
		}
		
		# other comment parts
	    }else{
		# a comment with the start of the comment on its own
		# line
		if( @$part[1] =~ /^\s*\n/s ){
		    $out .= qq(<p><pre>@$part[1]</pre></p>), "\n";

		# any other comment with newlines in it are treated as 
		# normal paragraphs
		}elsif( @$part[1] =~ /\n/s ){
		    $out .= qq(<p>@$part[1]</p>), "\n";

		# and single-line comments are headers
		}else{
		    $out .= qq(<h$l>@$part[1]</h$l>), "\n";
		    $l = 2;
		}
	    }
	    
	}elsif( @$part[0] eq '!ELEMENT' ){
	    @$part[1] = replace_ent(@$part[1]);
	    @$part[1] =~ /([\w.-]+)\s+(.+)/;
		my $tag = $1;
		my $class = "";
		my $contents = '<b class="empty">EMPTY</b>';
		if ($2 =~ /EMPTY/) {
			$class = "empty";
		} else {
			$contents = add_elem_href( $2 );
		}
	    $out .= qq(<hr />\n);
	    $out .= qq(<h3 id="$tag" class="$class"><code>&lt;$tag /&gt;</code></h3>\n);

	    my $in = "";
	    $in = add_elem_href( join ", ", @{$rev{$tag}}) if defined $rev{$tag};
	    $out .= qq(allowed in: <code>$in</code><br>\n) if $in;
	    $out .= qq(children: <code>$contents</code>);
	# unless $class eq "empty";

		$desc{$tag} =~ s/\n+$//g;
		$desc{$tag} =~ s/\n\n/\<br\/\>\<br\/\>/g;

	    $out .= qq(<p>$desc{$tag}</p>\n);


	}elsif( @$part[0] eq '!ATTLIST' ){

	    @$part[1] = replace_ent(@$part[1]);

	    # remove the first word (should be the same as the element)
	    @$part[1] =~ s/(\w+)\b\s*//;
	    my $elem = $1;

            # well, don't bother if there's nothing here
	    next if @$part[1] =~ /^\s*$/;

		if ( qq($elem) ne qq($prevelem) ) {

		    print "<!-- @$part[1] -->\n";

		    $out .= qq(<h4><code>$elem</code> Attributes</h4>\n);

		$prevelem = $elem;
		}

		    $out .= qq(<dl>\n);

	    # put all the attributes in a definition list

	    while( @$part[1] =~ /(\w+)\s+(.+)\s+(\S+)\s*/gs ){
		my ($att, $type, $value) = ($1, $2, $3);

		my $class = ($value eq "#REQUIRED") ? "required" : "implied";
		my $opt = ($class eq "required") ? "YES": "NO";
		
		$out .= qq(<dt class="$class">$att</dt>\n);
		$out .= qq(<dd>Type: <code>$type</code><br/>Required: $opt<br/>$desc{"$elem/$att"}</dd>\n);

	    } 
	    $out .= qq(</dl>\n);

	}elsif( @$part[0] eq '!ENTITY' ){

	    if( @$part[1] =~ /[\%]\s+(\w+)\s+(\'|\")(.+)(\'|\")/s ){
		$ents{$1} = replace_ent($3);
	    }elsif( @$part[1] =~ /[\%]\s+(\w+)\s+SYSTEM\s+(\'|\")(.+?)(\'|\")/s ){
		my $ent  = $1;
		my $file = $2;
		$file =~ s/\.dtd$/.html/;
		$out .= qq(<a href="$file">\%$ent;</a><br>\n);
	    }else{
		warn "unknown entity: @$part[1]\n";
	    }
	}
    }

	$out =~ s/\@todo/<b class="empty">\@todo<\/b>/g;

    return $out;
}

# finds all words in the string and links them appropriately
sub add_elem_href($){
    my ($text) = @_;

    $text =~ s/(\b(?!PCDATA\b)[\w.-]+\b)/<a href="#$1">$1<\/a>/g;
    
    return $text;
}

# replaces entities in the given text
sub replace_ent($){
    my ( $text ) = @_;

    foreach my $ent( keys %ents ){
	$text =~ s/\%$ent\;/$ents{$ent}/g;
    }

    return $text;
}

# returns a HTML-escaped string
sub escaped($){
    my ( $text ) = @_;

    $text =~ s/\&/&amp;/g;
    $text =~ s/\</&lt;/g;
    $text =~ s/\>/&gt;/g;

    return $text;
}


sub format_usage(){

    die<<FORMAT;
Use the following format:

<!-- foo: foo-element description
          attribute  - attribute description
          attribute2 - another attribute description
-->
<!ELEMENT foo CHILDREN>
<!ATTLIST foo
          attribute  TYPE  #REQUIRED
          attribute2 CDATA #IMPLIED>

other comments are treated as follows:

<!-- single-line comments are headers -->
<!-- multi-line comments are put in 
     html paragraph tags -->

<!-- 
   preformatted text goes in comments 
   starting on a seprate line
-->
FORMAT

}

sub usage(){
    die<<USAGE;
usage: $0 [options] file.dtd

Outputs an HTML interpretation of the DTD. Comments within the DTD allow for
documentation. 

options:
    -f          prints format help
    -h          this help
    -V          print version information

Copyright(C) 2002-2004 Steve Pomeroy <steve\@staticfree.info>
Licensed under the GNU GPL. See documentation for complete details.

for more info, see $url
USAGE

}
