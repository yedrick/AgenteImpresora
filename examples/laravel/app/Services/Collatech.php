<?php

namespace App\Services;

use App\Models\CollatechAgente;
use App\Models\CollatechTrabajo;
use DateTimeInterface;
use InvalidArgumentException;

/**
 * Imprimir en las impresoras de un local desde cualquier parte de tu app.
 *
 *     $local = CollatechAgente::where('nombre', 'Centro')->first();
 *
 *     Collatech::imprimir($local, 'cocina', [
 *         'title' => 'COMANDA #42',
 *         'lines' => [['text' => '2 x Cafe con leche', 'size' => 'tall']],
 *     ]);
 *
 * Los datos son los mismos que acepta el agente en POST /api/print/ticket
 * (ver JSON-REFERENCIA.md); "printer" lo pone esta clase.
 */
class Collatech
{
    /** Tipos que entiende el agente. */
    public const TIPOS = ['ticket', 'text', 'template', 'html', 'layout', 'image', 'logo', 'raw'];

    /**
     * Encola un trabajo. Sale en cuanto el agente lo recoja (en unos 3
     * segundos si el local esta encendido).
     *
     * @param  DateTimeInterface|null  $caducaEn  si el agente no lo ha
     *   recogido para entonces, no se imprime. Para comandas, que no salgan
     *   horas tarde si el local estuvo apagado.
     */
    public static function imprimir(
        CollatechAgente $agente,
        string $estacion,
        array $datos,
        string $tipo = 'ticket',
        ?DateTimeInterface $caducaEn = null,
    ): CollatechTrabajo {
        if (! in_array($tipo, self::TIPOS, true)) {
            throw new InvalidArgumentException("Tipo '{$tipo}' desconocido. Validos: ".implode(', ', self::TIPOS));
        }
        $datos['printer'] = $estacion;

        return $agente->trabajos()->create([
            'estacion' => $estacion,
            'tipo' => $tipo,
            'datos' => $datos,
            'estado' => CollatechTrabajo::PENDIENTE,
            'caduca_en' => $caducaEn,
        ]);
    }

    /**
     * Igual que imprimir(), pero antes comprueba que se puede: si el local
     * esta apagado o la impresora sin papel, lanza la excepcion con el motivo
     * en vez de encolar algo que no va a salir.
     *
     * @throws ImpresoraNoDisponible
     */
    public static function imprimirSiSePuede(
        CollatechAgente $agente,
        string $estacion,
        array $datos,
        string $tipo = 'ticket',
        ?DateTimeInterface $caducaEn = null,
    ): CollatechTrabajo {
        if ($problema = $agente->problema($estacion)) {
            throw new ImpresoraNoDisponible($problema);
        }

        return self::imprimir($agente, $estacion, $datos, $tipo, $caducaEn);
    }

    /**
     * Espera a que el trabajo termine, hasta $segundos. Para cuando la
     * respuesta al usuario depende de que el ticket haya salido.
     */
    public static function esperar(CollatechTrabajo $trabajo, int $segundos = 15): CollatechTrabajo
    {
        $limite = microtime(true) + $segundos;
        while (! $trabajo->terminado() && microtime(true) < $limite) {
            usleep(500_000);
            $trabajo->refresh();
        }

        return $trabajo;
    }
}
