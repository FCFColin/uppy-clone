$files = Get-ChildItem 'd:\Project\多人网页游戏\backend\internal\game\*_test.go'
$total = 0
foreach ($f in $files) {
  $c = (Get-Content $f.FullName).Count
  $total += $c
  Write-Output ($f.Name + ': ' + $c)
}
Write-Output ('TOTAL: ' + $total)
