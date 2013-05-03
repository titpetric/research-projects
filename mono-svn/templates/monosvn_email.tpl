{body}
<hr size="1" noshade>

<pre><a style="margin:0" href="{settings.websvn}comp.php?repname={eval echo urlencode(trim(str_replace($settings['svn-base-dir'],"",$repository),"/"))}&path=%2F&compare[]=%2F@{eval echo $revision-1;}&compare[]=%2F@{revision}">WebSVN Diff</a></pre>
<br/>
<table cellspacing="0" cellpadding="0" width="100%" border="0">
{eval $opentd = false;}
{foreach $diff as $line}
{if count($diff)<1500 || (in_array(substr($line,0,4),array("--- ","+++ ")) && strpos($line," (rev ")!==false) || substr($line,0,3)=="@@ " || substr($line,0,10)=="=========="}
{if substr($line,0,1)=='-'}
{if $opentd}</pre></td></tr>
{/if}
<tr><td bgcolor="#FFA070"><pre style="margin : 0;">{line|htmlspecialchars}</pre></td>
{eval $opentd = false;}
{elseif substr($line,0,1)=='+'}
{if $opentd}</pre></td></tr>
{/if}
<tr><td bgcolor="#A0E0FF"><pre style="margin : 0;">{line|htmlspecialchars}</pre></td>
{eval $opentd = false;}
{else}
{if !$opentd}<tr><td><pre style="margin : 0;">{/if}{line|htmlspecialchars}<br/>{eval $opentd = true;}{/if}
{/if}
{/foreach}
{if $opentd}</pre></td></tr>
{/if}
</table>
<br />
<hr size="1" noshade><br />
{settings['from-team-signature']}