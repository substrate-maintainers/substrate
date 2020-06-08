package main

// managed by go generate; do not edit by hand

func indexTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<meta charset="utf-8">
<title>Instance Factory</title>
<body>
<h1>Instance Factory</h1>
{{- if .Error}}
<p class="error">{{.Error}}</p>
{{- end}}
<form method="GET">
<p>Provision a new EC2 instance in:
{{- range $i, $region := .Regions}}
<input name="region" type="submit" value="{{$region}}">
{{- end}}
</p>
</form>
<table border="1" cellpadding="2" cellspacing="2">
<tr>
    <th>Hostname</th>
    <th>Availability Zone</th>
    <th>Provision Time</th>
    <th>&nbsp;</th>
</tr>
</table>
<hr>
<pre>{{.Debug}}</pre>
</body>
</html>
`
}
