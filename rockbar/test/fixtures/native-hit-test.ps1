param([string]$WindowHandle,[int]$ViewportWidth,[int]$ViewportHeight,[string]$Points)
$ErrorActionPreference='Stop'
# Query only the owned Electron test window. No mouse input or window movement.
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class HitProbe {
 [StructLayout(LayoutKind.Sequential)] public struct Point { public int X,Y; }
 [StructLayout(LayoutKind.Sequential)] public struct Rect { public int Left,Top,Right,Bottom; }
 [DllImport("user32.dll")] public static extern bool GetClientRect(IntPtr window,out Rect rect);
 [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr window,ref Point point);
 [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("user32.dll",SetLastError=true)] public static extern IntPtr SendMessageTimeout(IntPtr window,uint message,IntPtr wParam,IntPtr lParam,uint flags,uint timeout,out UIntPtr result);
}
'@
[HitProbe]::SetThreadDpiAwarenessContext([IntPtr](-4)) | Out-Null
$handle=[IntPtr]([Convert]::ToInt64($WindowHandle,16))
$rect=New-Object HitProbe+Rect
if(-not [HitProbe]::GetClientRect($handle,[ref]$rect)){throw 'Invalid test window'}
$results=@(foreach($item in ($Points|ConvertFrom-Json)) {
 $point=New-Object HitProbe+Point
 $point.X=[int]($item.x*($rect.Right-$rect.Left)/$ViewportWidth)
 $point.Y=[int]($item.y*($rect.Bottom-$rect.Top)/$ViewportHeight)
 if(-not [HitProbe]::ClientToScreen($handle,[ref]$point)){throw 'ClientToScreen failed'}
 $packed=([long]($point.Y -band 65535) -shl 16) -bor ($point.X -band 65535)
 $hit=[UIntPtr]::Zero
 if([HitProbe]::SendMessageTimeout($handle,0x84,[IntPtr]::Zero,[IntPtr]$packed,2,2000,[ref]$hit) -eq [IntPtr]::Zero){throw 'Hit test timed out'}
 [long]$hit.ToUInt64()
})
ConvertTo-Json -Compress -InputObject $results
