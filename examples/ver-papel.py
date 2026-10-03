#!/usr/bin/env python3
"""Lee un volcado ESC/POS y lo muestra como saldria en el papel."""
import sys
d=open(sys.argv[1] if len(sys.argv)>1 else 'prueba/impresora-virtual.bin','rb').read()
A=48  # columnas a 80 mm
i=0; out=[]; linea=b''; st={'bold':False,'align':0,'size':1}
def volcar():
    global linea
    if not linea: return
    txt=linea.decode('cp850','replace'); linea=b''
    if st['size']>1: txt=' '.join(txt)
    pad={0:0,1:max(0,(A-len(txt))//2),2:max(0,A-len(txt))}[st['align']]
    marca=''
    if st['bold']: marca+=' «negrita»'
    if st['size']>1: marca+=' «doble»'
    out.append('│'+(' '*pad+txt).ljust(A)[:A]+'│'+marca)
while i<len(d):
    b=d[i]
    if b==0x1B and i+1<len(d):
        c=d[i+1]
        if c==0x40: out.append('┌'+'─'*A+'┐  (inicializar)'); i+=2; continue
        if c==0x61: volcar(); st['align']=d[i+2]; i+=3; continue
        if c==0x45: volcar(); st['bold']=bool(d[i+2]); i+=3; continue
        if c==0x64: volcar(); [out.append('│'+' '*A+'│') for _ in range(min(d[i+2],6))]; i+=3; continue
        if c==0x74: i+=3; continue
        if c==0x33: i+=3; continue
        if c in (0x32,0x4D): i+=2 if c==0x32 else 3; continue
        if c==0x7B: out.append(f'  (girado 180: {"si" if d[i+2] else "no"})'); i+=3; continue
        i+=2; continue
    if b==0x1D and i+1<len(d):
        c=d[i+1]
        if c==0x21: volcar(); st['size']=1+(d[i+2]>>4); i+=3; continue
        # Ajustes del codigo de barras: alto, grosor, donde va el texto
        # legible y con que fuente. Todos son de 3 bytes; tratarlos como de
        # 2 dejaba el parametro suelto y se imprimia como una letra.
        if c in (0x68,0x77,0x48,0x66): i+=3; continue
        if c==0x4C or c==0x57: i+=4; continue   # margen izquierdo, ancho
        if c==0x42: i+=3; continue
        if c==0x56:
            volcar(); out.append('└'+'─'*A+'┘'); out.append('   ✂ '+'- '*22+'CORTE'); out.append('')
            i+=4 if d[i+2]==66 else 3; continue
        if c==0x76 and i+2<len(d) and d[i+2]==0x30:
            volcar(); bw=d[i+4]+d[i+5]*256; h=d[i+6]+d[i+7]*256; n=bw*h; dat=d[i+8:i+8+n]
            out.append('│'+f'  (imagen {bw*8}x{h} puntos)'.ljust(A)[:A]+'│')
            paso=max(1,(bw*8)//A)
            for y in range(0,h,paso*2):
                f=''
                for x in range(0,bw*8,paso):
                    o=0
                    for dy in range(min(paso*2,h-y)):
                        for dx in range(paso):
                            px=x+dx; idx=(y+dy)*bw+px//8
                            if px<bw*8 and idx<len(dat) and (dat[idx]>>(7-px%8))&1: o+=1
                    f+='█' if o>paso*paso else ('▒' if o>paso//2 else ' ')
                out.append('│'+f.ljust(A)[:A]+'│')
            i+=8+n; continue
        if c==0x6B:
            # GS k tiene dos formas: con m de 0 a 6 los datos acaban en un
            # cero, y con m de 65 en adelante viene la longitud en el byte
            # siguiente. Dar por hecha la primera dejaba un byte suelto que
            # salia impreso como una "d" en medio del ticket.
            volcar(); m=d[i+2]
            if m >= 65:
                n=d[i+3]; dat=d[i+4:i+4+n]; i+=4+n
            else:
                j=i+3
                while j<len(d) and d[j]!=0: j+=1
                dat=d[i+3:j]; i=j+1
            txt=dat.decode('ascii','replace')
            out.append('│'+'  ▌▌▐▌ ▌▐▌▌ ▐▌ ▌▌▐ ▌▌▐▌ ▌'.ljust(A)[:A]+'│')
            out.append('│'+f'  codigo de barras: {txt}'.ljust(A)[:A]+'│')
            continue
        if c==0x28 and i+2<len(d) and d[i+2]==0x6B:
            # GS ( k: cabecera de 5 bytes (GS ( k pL pH) y luego cn fn y los
            # datos. La longitud cuenta desde cn, asi que el comando entero
            # ocupa 5 + pL+pH*256. Antes se saltaba uno de menos y el byte
            # suelto salia como una "d" en mitad del ticket.
            ln=d[i+3]+d[i+4]*256
            if i+5<len(d) and d[i+6]==0x50:      # fn 80: guardar datos
                txt=d[i+8:i+5+ln].decode('ascii','replace')
                for l in ('  ▛▀▀▜','  ▌██▐  QR  '+txt,'  ▙▄▄▟'):
                    out.append('│'+l.ljust(A)[:A]+'│')
            i+=5+ln; continue
        i+=2; continue
    if b==0x0A: volcar(); i+=1; continue
    linea+=bytes([b]); i+=1
volcar()
print('\n'.join(out))
