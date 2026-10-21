<?php

namespace App\Services;

use RuntimeException;

/** La impresora o el local no estan en condiciones de imprimir. */
class ImpresoraNoDisponible extends RuntimeException
{
}
