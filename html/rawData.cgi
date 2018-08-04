#!/usr/bin/perl
#

use CGI qw(:all);
$q = new CGI;

$iam = $q->self_url;

$post = 0;
$get  = 0;
$post = 1 if ( $q->request_method eq "POST" );
$get  = 1 if ( !$post );

print $q->header();
print $q->start_html(
  -title => "Raw Electric Grid and Solar Data",
  -head  => [
    meta( { -name => "viewport", -content => "width=device-width", "-user-scalable" => "yes" } ),
    meta( { -http_equiv => "refresh", -content => "2" } )
  ]
);
print $q->center( h2("Raw Electric Grid and Solar Data") ), $q->br();
open( DAT, "/home/httpd/html/solar/solarmonLiveData" );
my @dat = <DAT>;
close(DAT);
print $q->pre( join( " ", @dat ) );
print $q->end_html;
exit;

